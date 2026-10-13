package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRetirementRequiresRepeatingTheHostIDAndOnlyCallsDestinationAPI(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "POST" || r.URL.Path != "/api/v1/recovery/hosts/hst_a/retire" {
			t.Errorf("unexpected remote request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["confirm_host_id"] != "hst_a" {
			t.Errorf("confirmation: %v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"hst_a","retired":true}`))
	}))
	defer srv.Close()
	for _, confirm := range []string{"", "hst_b", "hst_a"} {
		e, out, errOut := newTestEnv(t)
		args := []string{"hosts", "retire", "hst_a", "--url", srv.URL}
		if confirm != "" {
			args = append(args, "--confirm-host-id", confirm)
		}
		code := dispatch(context.Background(), e, args)
		if confirm == "hst_a" {
			if code != exitOK {
				t.Fatalf("retire failed: %s", errOut)
			}
			if !strings.Contains(out.String(), "remote machine is untouched") {
				t.Fatal("retirement claimed remote cleanup")
			}
		} else if code == exitOK {
			t.Fatal("missing or wrong confirmation succeeded")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("retirement made %d requests, want one", calls.Load())
	}
}
