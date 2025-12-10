package reporter

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReporterPosts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/test" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("X-Worker-Token") != "tok" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Token: "tok", Client: srv.Client()}
	if err := c.post("/api/test", map[string]string{"a": "b"}); err != nil {
		t.Fatalf("post error: %v", err)
	}
}
