package cbz

import (
	"archive/zip"
	"bytes"
	"cmp"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"time"

	"github.com/adoublef/hey/internal/encoding/html"
	"github.com/adoublef/hey/internal/net/http/status"
	"go.adoublef.dev/xiota"
	"golang.org/x/sync/errgroup"
)

type Client struct {
	C *http.Client
}

func (c *Client) Series(ctx context.Context, u *url.URL, comp Compress) io.ReadCloser {
	g, ctx := errgroup.WithContext(ctx)

	urls := make(chan *url.URL)
	g.Go(func() error {
		defer close(urls)

		req, err1 := http.NewRequestWithContext(ctx, http.MethodGet, u.JoinPath("full-chapter-list").String(), nil)
		res, err2 := c.C.Do(req)
		if err := cmp.Or(err1, err2); err != nil {
			return fmt.Errorf("failed %q request with status: %v", req.URL, err)
		}
		defer res.Body.Close()

		if c := res.StatusCode; c != http.StatusOK {
			return fmt.Errorf("failed %q request with status: %w", req.URL, status.Code(c))
		}

		for u, err := range html.Anchors(res.Body) {
			if err != nil {
				return err
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case urls <- u:
			}
		}
		return nil
	})

	pr, pw := io.Pipe()
	g.Go(func() error {
		zw := zip.NewWriter(pw) // ~4kb
		defer zw.Close()

		// neither dst, not src implement [io.ReaderFrom] or [io.WriterTo]
		var buf = make([]byte, 32*1024)

		var count int
		for u := range urls {
			fh := &zip.FileHeader{
				Name:     strconv.Itoa(count) + ".zip",
				Method:   uint16(Store * 8),
				Modified: time.Now().UTC(),
			}
			w, err1 := zw.CreateHeader(fh)
			_, err2 := io.CopyBuffer(w, c.Chapter(ctx, u), buf)
			if err := cmp.Or(err1, err2); err != nil {
				return err
			}
			count++
		}
		return zw.Flush()
	})
	go func() { pw.CloseWithError(g.Wait()) }()
	return pr
}

func (c *Client) Chapter(ctx context.Context, u *url.URL, comp Compress) io.ReadCloser {
	g, ctx := errgroup.WithContext(ctx)

	urls := make(chan *url.URL)
	g.Go(func() error {
		defer close(urls)

		req, err1 := http.NewRequestWithContext(ctx, http.MethodGet, u.JoinPath("images").String(), nil)
		res, err2 := c.C.Do(req)
		if err := cmp.Or(err1, err2); err != nil {
			return fmt.Errorf("failed %q request with status: %v", req.URL, err)
		}
		defer res.Body.Close()

		if c := res.StatusCode; c != http.StatusOK {
			return fmt.Errorf("failed %q request with status: %w", req.URL, status.Code(c))
		}

		for url, err := range html.Images(res.Body) {
			if err != nil {
				return err
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case urls <- url:
			}
		}
		return nil
	})

	bufs := make(chan *bytes.Buffer) // todo: include name
	g.Go(func() error {
		defer close(bufs)

		g, ctx := errgroup.WithContext(ctx)
		g.SetLimit(1)

		for u := range urls {
			g.Go(func() error {
				req, err1 := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
				res, err2 := c.C.Do(req)
				if err := cmp.Or(err1, err2); err != nil {
					return fmt.Errorf("failed %q request with status: %v", req.URL, err)
				}
				rc := http.MaxBytesReader(nil, res.Body, res.ContentLength)
				defer rc.Close()

				if c := res.StatusCode; c != http.StatusOK {
					return fmt.Errorf("failed %q request with status: %w", req.URL, status.Code(c))
				}

				var buf bytes.Buffer
				// See https://destel.dev/blog/on-the-fly-content-type-detection-in-go
				if _, err := io.CopyN(&buf, rc, 512); err != nil {
					return err
				}
				switch ct := http.DetectContentType(buf.Bytes()); path.Dir(ct) { // get the top part
				case "image": // content-type can be spoofed
				default:
					return fmt.Errorf("unsupported image type: %q", ct)
				}
				_, err := io.Copy(&buf, rc)
				if err != nil {
					return err
				}

				select {
				case <-ctx.Done():
					return ctx.Err()
				case bufs <- &buf:
				}
				return nil
			})
		}

		return g.Wait()
	})

	pr, pw := io.Pipe()
	g.Go(func() error {
		zw := zip.NewWriter(pw)
		defer zw.Close()

		var count int
		for src := range bufs {
			fh := &zip.FileHeader{
				Name:     strconv.Itoa(count) + ".jpeg",
				Method:   uint16(Store * 8),
				Modified: time.Now().UTC(),
			}
			w, err1 := zw.CreateHeader(fh)
			_, err2 := io.Copy(w, src)
			if err := cmp.Or(err1, err2); err != nil {
				return err
			}
			count++
		}

		return zw.Flush()
	})

	go func() { pw.CloseWithError(g.Wait()) }()
	return pr
}

type Compress uint8

const (
	Store Compress = iota
	Deflate
)

var compression = [...]string{
	"store",
	"deflate",
}

func (s Compress) String() string {
	return xiota.Format(s, compression[:], Store, Deflate, 0)
}

func (s *Compress) UnmarshalText(p []byte) (err error) {
	*s, err = ParseCompress(string(p))
	return
}

func (s Compress) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

func ParseCompress(s string) (Compress, error) {
	return xiota.Parse[Compress](compression[:], s, 0)
}
