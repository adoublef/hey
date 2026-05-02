package httputil

import "net/http"

type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

func (h HandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ww := Wrap(w)
	err := h(ww, r)
	if err == nil || ww.Written() {
		return
	}
	if h, ok := err.(http.Handler); ok {
		h.ServeHTTP(ww, r)
		return
	}
	http.Error(ww, err.Error(), http.StatusInternalServerError)
}
