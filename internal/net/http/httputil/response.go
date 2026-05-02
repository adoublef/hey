package httputil

import "net/http"

type ResponseWriter interface {
	http.ResponseWriter
	Written() bool
}

type response struct {
	http.ResponseWriter
	status int
	size   int
}

// Write implements [ResponseWriter].
func (r *response) Write(b []byte) (int, error) {
	if !r.Written() {
		r.WriteHeader(http.StatusOK)
	}
	size, err := r.ResponseWriter.Write(b)
	r.size += size
	return size, err
}

// WriteHeader implements [ResponseWriter].
func (r *response) WriteHeader(statusCode int) {
	// Avoid panic if status code is not a valid HTTP status code
	if statusCode < 100 || statusCode > 999 {
		r.ResponseWriter.WriteHeader(500)
		r.status = 500
		return
	}

	r.ResponseWriter.WriteHeader(statusCode)
	r.status = statusCode
}

// Written implements [ResponseWriter].
func (r *response) Written() bool { return r.status != 0 }

func Wrap(w http.ResponseWriter) ResponseWriter {
	if ww, ok := w.(ResponseWriter); ok {
		return ww
	}
	return &response{w, 0, 0}
}
