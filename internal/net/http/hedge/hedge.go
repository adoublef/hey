package hedge

import "net/http"

var _ http.RoundTripper = (*Transport)(nil)

type Transport struct{}

// RoundTrip implements [http.RoundTripper].
func (t *Transport) RoundTrip(*http.Request) (*http.Response, error) {
	panic("unimplemented")
}
