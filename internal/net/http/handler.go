package http

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/adoublef/hey/internal/net/http/httputil"
	"github.com/adoublef/hey/internal/net/http/status"
)

type Server = http.Server

func IsServeClosed(err error) bool {
	return errors.Is(err, http.ErrServerClosed)
}

func Handler(db *sql.DB) http.Handler {
	mux := http.NewServeMux()
	h := func(pattern string, handler http.Handler) {
		mux.Handle(pattern, handler)
	}

	h("GET /", status.Code(http.StatusTeapot))
	h("GET /hey", handleHey())
	h("GET /ok", handleOk(db))
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
