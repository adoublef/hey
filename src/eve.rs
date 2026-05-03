mod error;

use crate::eve::error::Error;
use anyhow::Context;
use async_stream::try_stream;
use bytes::Bytes;
use csv_async::AsyncWriterBuilder;
use futures::{SinkExt as _, StreamExt as _, TryStreamExt as _, stream::BoxStream};
use http_json_stream::{JsonPart, JsonStream};
use serde::{Deserialize, Serialize};
use std::io;
use tokio::{
    sync::mpsc::{self},
    task::JoinSet,
};
use tokio_stream::wrappers::ReceiverStream;
use tokio_util::{
    io::{CopyToBytes, SinkWriter},
    sync::PollSender,
};
use url::Url;

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
pub struct Order {
    pub duration: i64,
    pub is_buy_order: bool,
    pub issued: String,
    pub location_id: i64,
    pub min_volume: i64,
    pub order_id: i64,
    pub price: f64,
    pub range: String,
    pub system_id: i64,
    pub type_id: i64,
    pub volume_remain: i64,
    pub volume_total: i64,
}

impl Order {
    pub fn to_record(&self) -> [String; 12] {
        [
            self.duration.to_string(),
            self.is_buy_order.to_string(),
            self.issued.clone(),
            self.location_id.to_string(),
            self.min_volume.to_string(),
            self.order_id.to_string(),
            self.price.to_string(),
            self.range.clone(),
            self.system_id.to_string(),
            self.type_id.to_string(),
            self.volume_remain.to_string(),
            self.volume_total.to_string(),
        ]
    }
}

#[derive(Debug, Default, Clone)]
pub struct Client {
    pub client: reqwest::Client,
}

impl Client {
    pub fn orders(&self, base: Url) -> BoxStream<'static, Result<Bytes>> {
        let mut set = JoinSet::<Result<()>>::new();

        let (tx, regions) = mpsc::channel(1);
        let client = self.client.clone();
        let url = base.clone();
        set.spawn(async move {
            let response = client
                .get(url.join("/v1/universe/regions")?)
                .send()
                .await?
                .error_for_status()?;

            let mut stream = JsonStream::<_, _, u32>::process(response, JsonPart::level(1))
                .map_err(|e| io::Error::new(io::ErrorKind::BrokenPipe, e.to_string()));
            while let Some(id) = stream.try_next().await? {
                let _ = tx.send(id).await?;
            }
            Ok(())
        });

        let (tx, queries) = mpsc::channel(1);
        let client = self.client.clone();
        let url = base.clone();
        set.spawn(async move {
            ReceiverStream::new(regions)
                .map(Ok)
                .try_for_each_concurrent(1, async |region| {
                    let last = client
                        .head(url.join(&format!("/v1/markets/{region}/orders"))?)
                        .send()
                        .await?
                        .error_for_status()?
                        .headers()
                        .get("x-pages")
                        .context("could not find x-pages header value")?
                        .to_str()?
                        .parse::<u32>()?;

                    for page in 1..=last {
                        tx.send((region, page)).await?
                    }
                    Ok(())
                })
                .await
        });

        let (tx, orders) = mpsc::channel(1);
        let client = self.client.clone();
        let url = base.clone();
        set.spawn(async move {
            ReceiverStream::new(queries)
                .map(Ok)
                .try_for_each_concurrent(1, async |(region, page)| {
                    let response = client
                        .get(url.join(&format!("/v1/markets/{region}/orders?page={page}"))?)
                        .send()
                        .await?
                        .error_for_status()?;

                    let mut stream =
                        JsonStream::<_, _, Order>::process(response, JsonPart::level(1))
                            .map_err(|_| io::Error::from(io::ErrorKind::BrokenPipe));
                    while let Some(order) = stream.try_next().await? {
                        tx.send(order).await?
                    }
                    Ok(())
                })
                .await
        });

        let (tx, bytes) = mpsc::channel(1);
        set.spawn(async move {
            let sink =
                PollSender::new(tx).sink_map_err(|_| io::Error::from(io::ErrorKind::BrokenPipe));
            let writer = SinkWriter::new(CopyToBytes::new(sink));

            let mut wri = AsyncWriterBuilder::new()
                .buffer_capacity(4 * 1 << 10) // default of the writer capacity
                .create_writer(writer);
            let mut orders = ReceiverStream::new(orders);
            while let Some(order) = orders.next().await {
                wri.write_record(order.to_record()).await?;
            }
            wri.flush().await?;
            Ok(())
        });

        try_stream! {
            let mut stream = ReceiverStream::new(bytes);
            while let Some(bytes) = stream.next().await {
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
