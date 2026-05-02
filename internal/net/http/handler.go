package http

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/adoublef/hey/internal/cbz"
	"github.com/adoublef/hey/internal/eve"
	"github.com/adoublef/hey/internal/machine"
	"github.com/adoublef/hey/internal/net/http/httputil"
	"github.com/adoublef/hey/internal/net/http/status"
	"github.com/google/uuid"
)

type Server = http.Server

var DefaultClient = http.DefaultClient

func IsServeClosed(err error) bool {
	return errors.Is(err, http.ErrServerClosed)
}

func Handler(machDB *machine.DB, eveClient *eve.Client, cbzClient *cbz.Client) http.Handler {
	mux := http.NewServeMux()
	f := func(pattern string, handler http.Handler) {
		mux.Handle(pattern, handler)
	}

	f("GET /hey", handleHey())
	f("GET /evetech/orders", handleOrders(eveClient))
	f("GET /cbz", handleCbz(cbzClient))
	f("POST /machines", handleAddMachine(machDB))
	f("GET /machines/{machine}", handleMachine(machDB))
	// runtime tracing endpoint?
	return mux
}

func handleHey() httputil.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		fmt.Fprintf(w, "Hey, 👋🏿!")
		return nil
	}
}

func handleOrders(c *eve.Client) httputil.HandlerFunc {
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

func handleCbz(c *cbz.Client) httputil.HandlerFunc {
	parse := func(_ http.ResponseWriter, r *http.Request) (*url.URL, cbz.Compress, error) {
		parsed, err1 := url.Parse(r.URL.Query().Get("series_url"))
		comp, err2 := cbz.ParseCompress(cmp.Or(r.URL.Query().Get("compress"), cbz.Store.String()))
		if err := cmp.Or(err1, err2); err != nil {
			return nil, cbz.Store, StatusBadRequest
		}
		// ensure path is formatted explicitly as /[series]/[id]
		path := strings.TrimPrefix(parsed.Path, "/")
		first, rest, more := strings.Cut(path, "/")
		if !more || first == "" || rest == "" || strings.Contains(rest, "/") {
			return nil, cbz.Store, StatusUnprocessableEntity
		}
		return parsed, comp, nil
	}
	return func(w http.ResponseWriter, r *http.Request) error {
		u, comp, err := parse(w, r)
		if err != nil {
			return fmt.Errorf("inavlid request: %w", err)
		}

		zr := c.Series(r.Context(), u, comp)
		defer zr.Close()

		h := w.Header()
		h.Set("Content-Type", "application/octet-stream")
		h.Set("Content-Disposition", "attachment; filename=\"cbz.zip\"")

		_, err = io.Copy(w, zr)
		return err
	}
}

func handleAddMachine(d *machine.DB) httputil.HandlerFunc {
	// parse json for extra metadata
	type response struct {
		ID uuid.UUID `json:"id"`
	}
	return func(w http.ResponseWriter, r *http.Request) error {
		ctx := r.Context()

		id, err := d.Add(ctx, machine.Meta{State: machine.StateCreating})
		if err != nil {
			// status is depenant on the error returned from the database
			return fmt.Errorf("failed to insert machine: %v: %w", err, status.Code(http.StatusFailedDependency))
		}
		// set the header here or elsewhere?
		return respond(w, r, response{id}, http.StatusCreated)
	}
}

func handleMachine(d *machine.DB) httputil.HandlerFunc {
	parse := func(_ http.ResponseWriter, r *http.Request) (uuid.UUID, error) {
		return uuid.Parse(r.PathValue("machine"))
	}

	type response struct {
		ID    uuid.UUID     `json:"id"`
		State machine.State `json:"state"`
	}
	return func(w http.ResponseWriter, r *http.Request) error {
		id, err := parse(w, r)
		if err != nil {
			return fmt.Errorf("failed to decode machine id: %v: %w", err, StatusBadRequest)
		}
		ctx := r.Context()

		mach, err := d.Machine(ctx, id)
		if err != nil {
			// status is depenant on the error returned from the database
			return fmt.Errorf("failed to find machine: %v: %w", err, status.Code(http.StatusFailedDependency))
		}

		return respond(w, r, response{mach.ID, mach.Meta.State}, http.StatusOK)
	}
}

var (
	StatusBadRequest          = status.Code(http.StatusBadRequest)
	StatusUnprocessableEntity = status.Code(http.StatusUnprocessableEntity)
)

// respond sets application/json as Content-Type and sends the payload to client.
func respond[V any](w http.ResponseWriter, _ *http.Request, v V, code int) error {
	w.WriteHeader(code)
	return json.NewEncoder(w).Encode(v)
}
