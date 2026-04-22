package httputil

import "net/http"

type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

func (h HandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	err := h(w, r)
	if err == nil {
		return
	}
	// ... we need to handle this
	// if we have a http.Handler, let's just use its method
	if h, ok := err.(http.Handler); ok {
		h.ServeHTTP(w, r)
		return
	}
	// if not, then we need to do some more
	// for now we will just return 500 and figure out an api later
}
