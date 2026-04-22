package http

import (
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

func Handler() http.Handler {
	mux := http.NewServeMux()
	h := func(pattern string, handler http.Handler) {
		mux.Handle(pattern, handler)
	}

	h("GET /", status.Code(http.StatusTeapot))
	h("GET /hey", handleHey())
	return mux
}

func handleHey() httputil.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		fmt.Fprintf(w, "Hey, 👋🏿!")
		return nil
	}
}
