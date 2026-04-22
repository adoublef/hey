package status

import (
	"fmt"
	"net/http"
)

type Code int

func (c Code) Error() string {
	return http.StatusText((int)(c))
}

func (c Code) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	// See https://go.dev/issue/66343.
	h.Del("Content-Length")

	// There might be content type already set, but we reset it to
	// text/plain for the error message.
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader((int)(c))
	fmt.Fprintln(w, c.Error())
}
