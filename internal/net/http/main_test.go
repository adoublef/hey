package http_test

import (
	"bytes"
	"cmp"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/adoublef/hey/internal/cbz"
	migrate "github.com/adoublef/hey/internal/database/postgres"
	"github.com/adoublef/hey/internal/eve"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

type migrator struct {
	pool *pgxpool.Pool
	fsys []fs.FS
}

func (m *migrator) up(ctx context.Context) error {
	conn, err := m.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	for _, fs := range m.fsys {
		err = errors.Join(err, migrate.Up(ctx, conn.Conn(), fs))
	}
	return err
}

func (m *migrator) down(ctx context.Context) error {
	conn, err := m.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	for _, fs := range m.fsys {
		err = errors.Join(err, migrate.Down(ctx, conn.Conn(), fs))
	}
	return err
}

var (
	dockerImagePostgres string
)

func init() {
	flag.StringVar(&dockerImagePostgres, "docker.image.postgres", "postgres:17-alpine", "postgres docker image")
}

func TestMain(m *testing.M) {
	ctx := context.Background()

	if err := setup(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	if err := teardown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	} else if code > 0 {
		os.Exit(code)
	}
}

var postgresContainer *postgres.PostgresContainer

func setup(ctx context.Context) (err error) {
	postgresContainer, err = postgres.Run(ctx, dockerImagePostgres, postgres.BasicWaitStrategies())
	return err
}

func teardown(ctx context.Context) error {
	return postgresContainer.Terminate(ctx)
}

func decode[V any](r io.Reader) (v V, err error) {
	err = json.NewDecoder(r).Decode(&v)
	if c, ok := r.(io.Closer); ok {
		err = errors.Join(err, c.Close())
	}
	return v, err
}

type client struct {
	client *http.Client
}

func (c *client) get(ctx context.Context, format string, v ...any) (*http.Response, error) {
	req, err1 := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(format, v...), nil)
	res, err2 := c.client.Do(req)
	return res, cmp.Or(err1, err2)
}

func (c *client) getJSON(ctx context.Context, format string, v ...any) (*http.Response, error) {
	req, err1 := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(format, v...), nil)
	res, err2 := c.client.Do(req)
	return res, cmp.Or(err1, err2)
}

func (c *client) postJSON(ctx context.Context, body any, format string, v ...any) (*http.Response, error) {
	p, _ := json.Marshal(body)
	req, err1 := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf(format, v...), bytes.NewReader(p))
	req.Header.Set("Content-Type", "application/json")
	res, err2 := c.client.Do(req)
	// todo: decode the response
	return res, cmp.Or(err1, err2)
}

func equal[T comparable](t testing.TB, got, want T) {
	t.Helper()

	if got != want {
		t.Errorf("got %v; want %v", got, want)
	}
}

func ok(t testing.TB, errs ...error) {
	t.Helper()

	if err := cmp.Or(errs...); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func testDB(t testing.TB) *pgxpool.Pool {
	t.Helper()
	ctx := t.Context()

	dsn, err := postgresContainer.ConnectionString(ctx)
	ok(t, err)

	cfg, err := pgxpool.ParseConfig(dsn)
	ok(t, err)
	cfg.MaxConns = 1

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	ok(t, err)

	t.Cleanup(pool.Close)
	return pool
}

func eveClient(t testing.TB, regions, max, orders int) (client *eve.Client, baseURL string) {
	t.Helper()

	mux := http.NewServeMux()

	{ // GET /v1/universe/regions
		const start = 10000
		var rr = make([]int, regions)
		for i := range regions {
			rr[i] = start + (i + 1)
		}
		p, err := json.Marshal(rr)
		if err != nil {
			t.Fail()
		}

		mux.HandleFunc("GET /v1/universe/regions", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", strconv.Itoa(len(p)))
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write(p); err != nil {
				t.Fail()
			}
		})
	}

	{ // HEAD /v1/markets/{id}/orders
		mux.HandleFunc("HEAD /v1/markets/{id}/orders", func(w http.ResponseWriter, r *http.Request) {
			_, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
			if err != nil {
				t.Fail()
			}
			w.Header().Set("x-pages", strconv.Itoa(max))
		})
	}

	{ // GET /v1/markets/{id}/orders
		var oo = make([]eve.Order, orders)
		for i := range orders {
			oo[i] = eve.Order{
				OrderID:      i + 1,
				IsBuyOrder:   false, // or true
				Issued:       "issued",
				LocationID:   1,
				MinVolume:    1,
				Price:        1,
				Range:        "range",
				SystemID:     1,
				TypeID:       1,
				VolumeRemain: 1,
				VolumeTotal:  1,
			}
		}
		p, err := json.Marshal(oo)
		if err != nil {
			t.Fail()
		}

		mux.HandleFunc("GET /v1/markets/{id}/orders", func(w http.ResponseWriter, r *http.Request) {
			_, err1 := strconv.ParseUint(r.PathValue("id"), 10, 64)
			_, err2 := strconv.ParseUint(r.URL.Query().Get("page"), 10, 64)
			if err := cmp.Or(err1, err2); err != nil {
				t.Fail()
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(p)))
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write(p); err != nil {
				t.Fail()
			}
		})
	}

	// See https://martin.baillie.id/wrote/gotchas-in-the-go-network-packages-defaults/
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)

	return &eve.Client{C: s.Client()}, s.URL
}

//go:embed testdata/*.html testdata/*.jpg
var embedFS embed.FS

func cbzClient(t testing.TB, chapters, images int) (client *cbz.Client, baseURL string) {
	t.Helper()

	funcMap := template.FuncMap{
		// See https://stackoverflow.com/a/22716709
		"N":    func(n int) []struct{} { return make([]struct{}, n) },
		"sub":  func(a, b int) int { return a - b },
		"inc":  func(i int) int { return i + 1 },
		"iota": func(i int) string { return strconv.Itoa(i) },
		"join": func(sep string, s ...string) string { return strings.Join(s, sep) },
		"url": func(base string, s ...string) string {
			u, err := url.JoinPath(base, s...)
			if err != nil {
				t.Fatal(err)
			}
			return u
		},
	}

	series, err1 := template.New("series.html").Funcs(funcMap).ParseFS(embedFS, "testdata/series.html")
	chapter, err2 := template.New("chapter.html").Funcs(funcMap).ParseFS(embedFS, "testdata/chapter.html")
	if err := cmp.Or(err1, err2); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()

	{ // GET /series/{series}/full-chapter-list
		mux.HandleFunc("GET /series/{series}/full-chapter-list", func(w http.ResponseWriter, r *http.Request) {
			// some formatted string
			_, err := strconv.ParseUint(r.PathValue("series"), 10, 64)
			if err != nil {
				t.Fatal(err)
			}

			w.Header().Set("Content-Type", "text/html")

			data := struct {
				N       int
				BaseURL string
			}{
				N:       chapters,
				BaseURL: baseURL,
			}
			if err := series.Execute(w, data); err != nil {
				t.Fatal(err)
			}
		})
	}
	{ // GET /chapters/{chapter}/images
		mux.HandleFunc("GET /chapters/{chapter}/images", func(w http.ResponseWriter, r *http.Request) {
			// some formatted string
			id, err := strconv.ParseUint(r.PathValue("chapter"), 10, 64)
			if err != nil {
				t.Fatal(err)
			}

			w.Header().Set("Content-Type", "text/html")

			data := struct {
				N       int
				Chapter int
				BaseURL string
			}{
				N:       images,
				Chapter: int(id),
				BaseURL: baseURL,
			}
			if err := chapter.Execute(w, data); err != nil {
				t.Fatal(err)
			}
		})
	}
	{ // GET /images/{image}
		mux.HandleFunc("GET /images/{image}", func(w http.ResponseWriter, r *http.Request) {
			// parse path?

			f, err1 := embedFS.Open("testdata/image.jpg") // base64
			fi, err2 := f.Stat()
			if err := cmp.Or(err1, err2); err != nil {
				t.Fatal(err)
			}
			defer f.Close()

			w.Header().Set("Content-Length", strconv.Itoa(int(fi.Size())))
			w.Header().Set("Content-Type", "image/jpeg")

			if r.Method != http.MethodHead {
				if _, err := io.Copy(w, f); err != nil {
					t.Fatal(err)
				}
			}
		})
	}

	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)

	return &cbz.Client{C: s.Client()}, s.URL
}
