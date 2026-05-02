package eve

import (
	"cmp"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/adoublef/hey/internal/encoding/json/jsonstream"
	"github.com/adoublef/hey/internal/net/http/status"
	"golang.org/x/sync/errgroup"
)

type Order struct {
	Duration     int     `json:"duration"`
	IsBuyOrder   bool    `json:"is_buy_order"`
	Issued       string  `json:"issued"`
	LocationID   int     `json:"location_id"`
	MinVolume    int     `json:"min_volume"`
	OrderID      int     `json:"order_id"`
	Price        float64 `json:"price"`
	Range        string  `json:"range"`
	SystemID     int     `json:"system_id"`
	TypeID       int     `json:"type_id"`
	VolumeRemain int     `json:"volume_remain"`
	VolumeTotal  int     `json:"volume_total"`
}

func (o Order) Record() [12]string {
	return [12]string{strconv.Itoa(o.Duration),
		strconv.FormatBool(o.IsBuyOrder),
		o.Issued,
		strconv.Itoa(o.LocationID),
		strconv.Itoa(o.MinVolume),
		strconv.Itoa(o.OrderID),
		strconv.FormatFloat(o.Price, 'f', -1, 64),
		o.Range,
		strconv.Itoa(o.SystemID),
		strconv.Itoa(o.TypeID),
		strconv.Itoa(o.VolumeRemain),
		strconv.Itoa(o.VolumeTotal),
	}
}

type Client struct {
	C *http.Client
}

func (c *Client) Orders(ctx context.Context, u *url.URL, header bool) io.ReadCloser {
	g, ctx := errgroup.WithContext(ctx)

	regions := make(chan uint64)
	g.Go(func() error {
		defer close(regions)

		url := fmt.Sprintf("%s://%s/v1/universe/regions", u.Scheme, u.Host)
		req, err1 := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		res, err2 := c.C.Do(req)
		if err := cmp.Or(err1, err2); err != nil {
			return fmt.Errorf("failed %q request with status: %v", req.URL, err)
		}
		defer res.Body.Close()

		if c := res.StatusCode; c != http.StatusOK {
			return fmt.Errorf("failed %q request with status: %w", req.URL, status.Code(c))
		}

		for id, err := range jsonstream.Decode[uint64](res.Body) {
			if err != nil {
				return err
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case regions <- id:
			}
		}
		return nil
	})

	type query struct{ region, page uint64 }
	queries := make(chan query)
	g.Go(func() error {
		defer close(queries)

		g, ctx := errgroup.WithContext(ctx)
		g.SetLimit(1)

		for id := range regions {
			g.Go(func() error {
				url := fmt.Sprintf("%s://%s/v1/markets/%d/orders", u.Scheme, u.Host, id)
				req, err1 := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
				res, err2 := c.C.Do(req)
				if err := cmp.Or(err1, err2); err != nil {
					return fmt.Errorf("failed %q request with status: %v", req.URL, err)
				}
				defer res.Body.Close()

				if c := res.StatusCode; c != http.StatusOK {
					return fmt.Errorf("failed %q request with status: %w", req.URL, status.Code(c))
				}

				max, err := strconv.ParseUint(res.Header.Get("x-pages"), 10, 32)
				if err != nil {
					return err
				}

				for i := range max {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case queries <- query{id, i + 1}:
					}
				}
				return nil
			})
		}

		return g.Wait()
	})

	records := make(chan [12]string)
	g.Go(func() error {
		defer close(records)

		g, ctx := errgroup.WithContext(ctx)
		g.SetLimit(1)

		for q := range queries {
			g.Go(func() error {
				url := fmt.Sprintf("%s://%s/v1/markets/%d/orders?page=%d", u.Scheme, u.Host, q.region, q.page)
				req, err1 := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
				res, err2 := c.C.Do(req)
				if err := cmp.Or(err1, err2); err != nil {
					return fmt.Errorf("failed %q request with status: %v", req.URL, err)
				}
				defer res.Body.Close()

				if c := res.StatusCode; c != http.StatusOK {
					return fmt.Errorf("failed %q request with status: %w", req.URL, status.Code(c))
				}

				for o, err := range jsonstream.Decode[Order](res.Body) {
					if err != nil {
						return err
					}
					select {
					case <-ctx.Done():
						return ctx.Err()
					case records <- o.Record():
					}
				}
				return nil
			})
		}

		return g.Wait()
	})

	pr, pw := io.Pipe()
	g.Go(func() error {
		cw := csv.NewWriter(pw)
		// add header
		if header {

		}
		for r := range records {
			if err := cw.Write(r[:]); err != nil {
				return err
			}
		}
		cw.Flush()
		return cw.Error()
	})
	go func() { pw.CloseWithError(g.Wait()) }()
	return pr
}
