package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// A client that creates a resource follows Location to read it back, so the
// header has to keep the mount prefix the request arrived on and escape the id.
func TestWriteCreatedNamesTheNewResourceBesideItsCollection(t *testing.T) {
	cases := []struct{ path, id, want string }{
		{"/api/v1/pools", "pool_1", "/api/v1/pools/pool_1"},
		{"/api/v1/pools/", "pool_1", "/api/v1/pools/pool_1"},
		{"/api/v1/transfers/uploads", "a b", "/api/v1/transfers/uploads/a%20b"},
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		writeCreated(w, httptest.NewRequest(http.MethodPost, c.path, nil), c.id, map[string]string{"id": c.id})
		if w.Code != http.StatusCreated {
			t.Errorf("%s: status %d, want 201", c.path, w.Code)
		}
		if got := w.Header().Get("Location"); got != c.want {
			t.Errorf("%s: Location %q, want %q", c.path, got, c.want)
		}
	}
}
