package tools

import (
	"net/http"

	httptransport "github.com/go-openapi/runtime/client"
)

// GetTLSTransport returns an HTTP transport. When insecure is true, it skips
// server certificate verification. The caller controls insecure via CLI/env.
func GetTLSTransport(insecure bool) (http.RoundTripper, error) {
	return httptransport.TLSTransport(httptransport.TLSClientOptions{
		InsecureSkipVerify: insecure,
	})
}
