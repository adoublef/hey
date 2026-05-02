package http_test

import (
	"archive/zip"
	"context"
	"encoding/csv"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adoublef/hey/internal/cbz"
	"github.com/adoublef/hey/internal/eve"
	"github.com/adoublef/hey/internal/machine"
	. "github.com/adoublef/hey/internal/net/http"
	"github.com/krolaw/zipstream"
)

func TestHandler_handleHey(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		ctx := t.Context()

		c, url := testClient(t, nil, nil, nil)

		res, err := c.get(ctx, "%s/hey", url)
		ok(t, err)
		equal(t, res.StatusCode, http.StatusOK)
		// read the body
		p, err := io.ReadAll(res.Body)
		ok(t, err)
		equal(t, string(p), "Hey, 👋🏿!")
		ok(t, res.Body.Close())
	})
}

func TestHandler_handleMachine(t *testing.T) {
	// t.Parallel() // need to solve the migration issue here

	t.Run("OK", func(t *testing.T) {
		ctx := t.Context()

		// deps
		var (
			pool = testDB(t)

			// do the migration here
			d = &machine.DB{
				RWC: pool,
			}

			m = &migrator{
				pool: pool,
				fsys: []fs.FS{machine.FS},
			}
		)

		err := m.up(ctx)
		ok(t, err)
		t.Cleanup(func() { m.down(context.Background()) })

		c, url := testClient(t, d, nil, nil)

		// post json
		res, err := c.postJSON(ctx, nil, "%s/machines", url)
		ok(t, err)
		equal(t, res.StatusCode, http.StatusCreated)
		created, err := decode[struct {
			ID string `json:"id"`
		}](res.Body)
		ok(t, err)

		// fix helper
		res, err = c.getJSON(ctx, "%s/machines/%s", url, created.ID)
		ok(t, err)
		equal(t, res.StatusCode, http.StatusOK)
		found, err := decode[struct {
			ID    string        `json:"id"`
			State machine.State `json:"state"`
		}](res.Body)
		equal(t, found.ID, created.ID)
		equal(t, found.State, machine.StateCreating)
	})
}

func TestHandler_handleCSV(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		ctx := t.Context()

		var (
			numRegions        = 1 << 0
			numPages          = 1 << 0
			numOrders         = 1 << 0
			eveClient, apiURL = eveClient(t, numOrders, numPages, numOrders)
		)

		c, url := testClient(t, nil, eveClient, nil)

		res, err := c.get(ctx, "%s/evetech/orders?base_url=%s", url, apiURL)
		ok(t, err)

		equal(t, res.StatusCode, http.StatusOK)
		equal(t, res.Header.Get("Content-Type"), "text/csv")
		// check content-disposition

		cr := csv.NewReader(res.Body)
		cr.ReuseRecord = true

		set := map[string]int{}
	LOOP:
		for {
			rr, err := cr.Read()
			if err == io.EOF {
				break LOOP
			}
			// check the size
			ok(t, err)
			equal(t, len(rr), 12)
			set[rr[5]]++ // orderId is unique
		}
		ok(t, res.Body.Close())

		// do we include the header?
		equal(t, len(set), numRegions)
		for _, n := range set {
			equal(t, n, 0+(numPages*numOrders)) // todo: include the header
		}
	})
}

func TestHandler_handleZIP(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		ctx := t.Context()

		var (
			numChapters       = 1 << 0
			numImages         = 1 << 0
			cbzClient, apiURL = cbzClient(t, numChapters, numImages)
		)

		c, url := testClient(t, nil, nil, cbzClient)

		res, err := c.get(ctx, "%s/cbz?series_url=%s/series/1", url, apiURL)
		ok(t, err)
		equal(t, res.StatusCode, http.StatusOK) // stream means this is always going to be the case

		// stream zip
		zr := zipstream.NewReader(res.Body) // ~4kb

		var i int
		for {
			fh, err := zr.Next()
			if err == io.EOF {
				break
			}
			i++
			ok(t, err)
			// is another file inside
			equal(t, len(fh.Name) > 0 && fh.Name[len(fh.Name)-1] == '/', false)
			// method is store
			equal(t, fh.Method, zip.Store)

			// internal zip
			r := zipstream.NewReader(zr)
			var j int
			for {
				fh, err := r.Next()
				if err == io.EOF {
					break
				}
				j++
				ok(t, err)
				// is another file inside
				equal(t, len(fh.Name) > 0 && fh.Name[len(fh.Name)-1] == '/', false)
				// method is store
				equal(t, fh.Method, zip.Store)

				n, err := io.Copy(io.Discard, r)
				ok(t, err)
				equal(t, n, 86387) // equal(t, n, 20028)
			}
			// count number of images
			equal(t, j, numImages)
		}
		equal(t, i, numChapters)
	})
}

func testClient(t testing.TB, machDB *machine.DB, eveClient *eve.Client, cbzClient *cbz.Client) (*client, string) {
	t.Helper()

	// start a new db for this + run migration

	s := httptest.NewServer(Handler(machDB, eveClient, cbzClient))
	return &client{s.Client()}, s.URL
}
