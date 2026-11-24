package okdatav1server

import (
	"errors"
	"net/http"
	"strings"

	ht "github.com/ogen-go/ogen/http"
)

type queryClient struct {
	ht.Client
}

func (c queryClient) Do(req *http.Request) (*http.Response, error) {
	if !isMeteredDataQuery(req.Method, req.URL.Path) {
		return c.Client.Do(req)
	}

	keys := req.Header.Values("Idempotency-Key")
	if len(keys) != 1 || !validQueryKey(keys[0]) {
		return nil, errors.New("Idempotency-Key must contain 1 to 255 visible ASCII characters")
	}

	// Preserve explicit caller replay without allowing transparent body resubmission.
	r := req.WithContext(req.Context())
	r.GetBody = nil
	return c.Client.Do(r)
}

func isMeteredDataQuery(method, path string) bool {
	_, resource, found := strings.Cut(path, "/api/v1/")
	if !found || (method != http.MethodGet && method != "QUERY" && method != http.MethodPost) {
		return false
	}
	collection, _, _ := strings.Cut(resource, "/")
	switch collection {
	case "enablement", "capabilities", "requests":
		return false
	default:
		return true
	}
}

func validQueryKey(key string) bool {
	if len(key) == 0 || len(key) > 255 {
		return false
	}
	for i := 0; i < len(key); i++ {
		if key[i] < '!' || key[i] > '~' {
			return false
		}
	}
	return true
}
