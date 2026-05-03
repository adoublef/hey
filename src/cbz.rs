mod error;

use anyhow::anyhow;
use async_stream::try_stream;
use async_zip::{Compression, ZipEntryBuilder, tokio::write::ZipFileWriter};
use bytes::Bytes;
use chrono::Utc;
use futures::{SinkExt as _, StreamExt, TryStreamExt, stream::BoxStream};
use http_body_util::{BodyDataStream, Limited};
use mimetype_detector::{detect_with_limit, equals_any};
use std::io;
use tokio::{
    sync::mpsc,
    task::{JoinSet, spawn_blocking},
};
use tokio_stream::wrappers::ReceiverStream;
use tokio_util::{
    compat::{FuturesAsyncReadCompatExt as _, FuturesAsyncWriteCompatExt as _},
    io::{CopyToBytes, SinkWriter, StreamReader, SyncIoBridge},
    sync::PollSender,
};
use url::Url;

use crate::{
    cbz::error::Error,
    encoding::html::{anchors, images},
};

#[derive(Debug, Default, Clone)]
pub struct Client {
    pub client: reqwest::Client,
}

impl Client {
    pub fn series(&self, mut series_url: Url) -> BoxStream<'static, Result<Bytes>> {
        let mut set = JoinSet::<Result<()>>::new();

        let (tx, urls) = mpsc::channel(1);
        let client = self.client.clone();
        set.spawn(async move {
            series_url
                .path_segments_mut()
                .map_err(|_| anyhow!("Invalid path segments"))?
                .push("full-chapter-list");

            let stream = client
                .get(series_url)
                .send()
                .await?
                .error_for_status()?
                .bytes_stream()
                .map_err(|e| io::Error::new(io::ErrorKind::BrokenPipe, e));
            let stream = StreamReader::new(stream);

            let fut = spawn_blocking(move || {
                let stream = SyncIoBridge::new(stream);
                for res in anchors(stream) {
                    let res = res?; // validate url parts
                    tx.blocking_send(res)?;
                }
                Ok::<_, Error>(())
            });
            Ok(fut.await??)
        });

        // get all chapter_urls from series stream
        // stream into a zip
        let (tx, mut bytes) = mpsc::channel(1);
        let this = self.clone();
        set.spawn(async move {
            let sink =
                PollSender::new(tx).sink_map_err(|_| io::Error::from(io::ErrorKind::BrokenPipe));
            let writer = SinkWriter::new(CopyToBytes::new(sink));
            let mut writer = ZipFileWriter::with_tokio(writer).force_zip64();

            let mut urls = ReceiverStream::new(urls).enumerate();
            while let Some((ix, chapter_url)) = urls.next().await {
                let mut chapter_stream = StreamReader::new(
                    this.chapter(chapter_url)
                        .map_err(|e| io::Error::new(io::ErrorKind::BrokenPipe, e)),
                );

                let mut chapter_entry = writer
                    .write_entry_stream(
                        ZipEntryBuilder::new(
                            format!("chapter-{ix}.zip").into(),
                            Compression::Stored,
                        )
                        .last_modification_date(Utc::now().into()),
                    )
                    .await?
                    .compat_write();

                tokio::io::copy(&mut chapter_stream, &mut chapter_entry).await?;
                chapter_entry.into_inner().close().await?;
            }

            _ = writer.close().await?;
            Ok(())
        });

        try_stream! {
            while let Some(bytes) = bytes.recv().await {
                yield bytes
            }
            while let Some(res) = set.join_next().await {
                res??
            }
        }
        .boxed()
    }

    pub fn chapter(&self, mut chapter_url: Url) -> BoxStream<'static, Result<Bytes>> {
        let mut set = JoinSet::<Result<()>>::new();

        let (tx, urls) = mpsc::channel::<Url>(1);
        let client = self.client.clone();
        set.spawn(async move {
            chapter_url
                .path_segments_mut()
                .map_err(|_| anyhow!("Invalid path segments"))?
                .push("images");

            let stream = client
                .get(chapter_url)
                .send()
                .await?
                .error_for_status()?
                .bytes_stream()
                .map_err(|e| io::Error::new(io::ErrorKind::BrokenPipe, e));
            let stream = StreamReader::new(stream);

            let fut = spawn_blocking(move || {
                let stream = SyncIoBridge::new(stream);
                for res in images(stream) {
                    let res = res?; // validate url parts
                    tx.blocking_send(res)?;
                }
                Ok::<_, Error>(())
            });
            Ok(fut.await??)
        });

        // get all image_bufs from image_url
        let (tx, image_bufs) = mpsc::channel(1);
        let client = self.client.clone();
        set.spawn(async move {
            ReceiverStream::new(urls)
                .map(Ok)
                .try_for_each_concurrent(1, async |image_url| {
                    let response = client.get(image_url).send().await?.error_for_status()?;
                    let content_length = response.content_length().unwrap_or(0);
                    let body = reqwest::Body::from(response);
                    let limited_body = Limited::new(body, content_length as usize);
                    let stream = BodyDataStream::new(limited_body)
                        .map_err(|_| io::Error::from(io::ErrorKind::BrokenPipe));
                    let mut reader = StreamReader::new(stream);

                    let mut buf = Vec::new();
                    tokio::io::copy(&mut reader, &mut buf).await?;

                    // TODO: i want to detect _before_ reading the whole content since we only need
                    let mime_type = detect_with_limit(&buf, 512).mime();
                    if !equals_any(
                        mime_type,
                        &[
                            mime::IMAGE_JPEG.essence_str(),
                            mime::IMAGE_PNG.essence_str(),
                        ],
                    ) {
                        return Err(anyhow!("Invalid mime type"))?;
                    };
                    Ok(tx.send(buf).await?)
                })
                .await
        });

        let (tx, mut bytes) = mpsc::channel(1);
        set.spawn(async move {
            let sink =
                PollSender::new(tx).sink_map_err(|_| io::Error::from(io::ErrorKind::BrokenPipe));
            let writer = SinkWriter::new(CopyToBytes::new(sink));
            let mut writer = ZipFileWriter::with_tokio(writer).force_zip64();

            let mut image_bufs = ReceiverStream::new(image_bufs).enumerate();
            while let Some((ix, image_buf)) = image_bufs.next().await {
                // Compat<&[u8]>
                let mut image_entry = writer
                    .write_entry_stream(
                        ZipEntryBuilder::new(format!("image-{ix}.png").into(), Compression::Stored)
                            .uncompressed_size(image_buf.len() as u64) // 86387
                            .last_modification_date(Utc::now().into()),
                    )
                    .await?
                    .compat_write();

                tokio::io::copy(&mut image_buf.compat(), &mut image_entry).await?;
                image_entry.into_inner().close().await?;
            }

            _ = writer.close().await?;
            Ok(())
        });

        try_stream! {
            while let Some(bytes) = bytes.recv().await {
                yield bytes
            }
            while let Some(res) = set.join_next().await {
                res??
            }
        }
        .boxed()
    }
}

type Result<T, E = Error> = core::result::Result<T, E>;
