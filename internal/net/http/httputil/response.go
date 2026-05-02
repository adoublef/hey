package httputil

import (
	"io"
	"net/http"
)

type ResponseWriter interface {
	http.ResponseWriter
	Written() bool
}

type response struct {
	http.ResponseWriter
	status int
	size   int
}

// ReadFrom implements [ResponseWriter].
func (w *response) ReadFrom(r io.Reader) (int64, error) {
	if !w.Written() {
		w.WriteHeader(http.StatusOK)
	}
	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		size, err := rf.ReadFrom(r)
		w.size += int(size)
		return size, err
	}
	// use the fallback
	return io.Copy(writerOnly{w}, r)
}

type writerOnly struct {
	io.Writer
}

// Write implements [ResponseWriter].
func (w *response) Write(b []byte) (int, error) {
	if !w.Written() {
		w.WriteHeader(http.StatusOK)
	}
	size, err := w.ResponseWriter.Write(b)
	w.size += size
	return size, err
}

// WriteHeader implements [ResponseWriter].
func (w *response) WriteHeader(statusCode int) {
	// Avoid panic if status code is not a valid HTTP status code
	if statusCode < 100 || statusCode > 999 {
		w.ResponseWriter.WriteHeader(500)
		w.status = 500
		return
	}

	w.ResponseWriter.WriteHeader(statusCode)
	w.status = statusCode
}

// Written implements [ResponseWriter].
func (w *response) Written() bool { return w.status != 0 }

func Wrap(w http.ResponseWriter) ResponseWriter {
	if ww, ok := w.(ResponseWriter); ok {
		return ww
	}
	return &response{w, 0, 0}
}
