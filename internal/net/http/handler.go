package http

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/adoublef/hey/internal/cbz"
	"github.com/adoublef/hey/internal/eve"
	"github.com/adoublef/hey/internal/net/http/httputil"
	"github.com/adoublef/hey/internal/net/http/status"
)

type Server = http.Server

var DefaultClient = http.DefaultClient

func IsServeClosed(err error) bool {
	return errors.Is(err, http.ErrServerClosed)
}

func Handler(dbConn *sql.DB, eveClient *eve.Client, cbzClient *cbz.Client) http.Handler {
	mux := http.NewServeMux()
	f := func(pattern string, handler http.Handler) {
		mux.Handle(pattern, handler)
	}

	f("GET /hey", handleHey())
	f("GET /ok", handleOk(dbConn))
	f("GET /csv", handleCSV(eveClient))
	f("GET /zip", handleZIP(cbzClient))
	return mux
}

func handleHey() httputil.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		fmt.Fprintf(w, "Hey, 👋🏿!")
		return nil
	}
}

func handleOk(db *sql.DB) httputil.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		return db.QueryRowContext(r.Context(), "SELECT 1").Err()
	}
}

func handleCSV(c *eve.Client) httputil.HandlerFunc {
	parse := func(_ http.ResponseWriter, r *http.Request) (base *url.URL, hasHeader bool, err error) {
		u, err := url.Parse(r.URL.Query().Get("base_url"))
		return u, false, err
	}
	return func(w http.ResponseWriter, r *http.Request) error {
		u, _, err := parse(w, r)
		if err != nil {
			return fmt.Errorf("invalid request: %v: %w", err, StatusBadRequest)
		}

		cr := c.Orders(r.Context(), u)
		defer cr.Close()

		h := w.Header()
		h.Set("Content-Type", "text/csv")
		h.Set("Content-Disposition", "attachment; filename=\"evetech.csv\"")

		_, err = io.Copy(w, cr)
		return err
	}
}

func handleZIP(c *cbz.Client) httputil.HandlerFunc {
	parse := func(_ http.ResponseWriter, r *http.Request) (*url.URL, error) {
		parsed, err := url.Parse(r.URL.Query().Get("series_url"))
		if err != nil {
			return nil, StatusBadRequest
		}
		// ensure path is formatted explicitly as /[series]/[id]
		path := strings.TrimPrefix(parsed.Path, "/")
		first, rest, more := strings.Cut(path, "/")
		if !more || first == "" || rest == "" || strings.Contains(rest, "/") {
			return nil, StatusUnprocessableEntity
		}
		return parsed, nil
	}
	return func(w http.ResponseWriter, r *http.Request) error {
		u, err := parse(w, r)
		if err != nil {
			return fmt.Errorf("inavlid request: %w", err)
		}

		zr := c.Series(r.Context(), u)
		defer zr.Close()

		h := w.Header()
		h.Set("Content-Type", "application/octet-stream")
		h.Set("Content-Disposition", "attachment; filename=\"cbz.zip\"")

		_, err = io.Copy(w, zr)
		return err
	}
}

var (
	StatusBadRequest          = status.Code(http.StatusBadRequest)
	StatusUnprocessableEntity = status.Code(http.StatusUnprocessableEntity)
)
