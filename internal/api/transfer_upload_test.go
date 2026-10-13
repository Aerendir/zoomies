package api

import (
	"bytes"
	"context"
	"github.com/eyupio/zoomies/internal/auth"
	"github.com/eyupio/zoomies/internal/store"
	"github.com/go-chi/chi/v5"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func startUpload(t *testing.T, h *harness, cookie string, size int64) *transferUpload {
	t.Helper()
	t.Cleanup(h.api.stopTransferUploads)
	r := h.do(request{method: http.MethodPost, path: "/api/v1/transfers/uploads", cookie: cookie, body: map[string]int64{"size": size}})
	r.mustStatus(t, http.StatusCreated, "create upload")
	u := &transferUpload{}
	r.into(t, u)
	return u
}
func TestTransferChunksAreBoundedOwnedAndSafeToReplay(t *testing.T) {
	h := newHarness(t)
	cookie := operatorCookie(h)
	u := startUpload(t, h, cookie, 6)
	path := "/api/v1/transfers/uploads/" + u.ID
	send := func(offset string, body string) *response {
		return h.do(request{method: http.MethodPost, path: path + "/chunks?offset=" + offset, cookie: cookie, rawBody: body, headers: map[string]string{"Content-Type": "application/octet-stream"}})
	}
	send("0", "abc").mustStatus(t, http.StatusOK, "first chunk")
	send("0", "abc").mustStatus(t, http.StatusOK, "identical replay")
	send("0", "xyz").mustStatus(t, http.StatusConflict, "changed replay")
	send("4", "d").mustStatus(t, http.StatusConflict, "out of order")
	h.do(request{method: http.MethodPost, path: path + "/verify", cookie: cookie, body: map[string]string{"passphrase": "archive phrase"}}).mustStatus(t, http.StatusConflict, "incomplete archive")
	send("3", string(bytes.Repeat([]byte{'x'}, transferChunkBytes+1))).mustStatus(t, http.StatusRequestEntityTooLarge, "oversized chunk")
	other := operatorCookie(h)
	h.do(request{method: http.MethodGet, path: path, cookie: other}).mustStatus(t, http.StatusNotFound, "other operator cannot read upload")
	h.do(request{method: http.MethodPost, path: path + "/chunks?offset=3", cookie: other, rawBody: "def"}).mustStatus(t, http.StatusNotFound, "other operator cannot append")
	send("3", "def").mustStatus(t, http.StatusOK, "last chunk")
	h.do(request{method: http.MethodDelete, path: path, cookie: cookie}).mustStatus(t, http.StatusNoContent, "cancel")
	send("6", "x").mustStatus(t, http.StatusNotFound, "cancelled upload")
}

type brokenChunk struct{ sent bool }

func (b *brokenChunk) Read(p []byte) (int, error) {
	if b.sent {
		return 0, io.ErrUnexpectedEOF
	}
	b.sent = true
	return copy(p, "abc"), nil
}
func (*brokenChunk) Close() error { return nil }
func TestAnInterruptedTransferChunkDoesNotCommitPartialBytes(t *testing.T) {
	h := newHarness(t)
	cookie := operatorCookie(h)
	u := startUpload(t, h, cookie, 6)
	h.api.transfers.mu.Lock()
	upload := h.api.transfers.items[u.ID]
	h.api.transfers.mu.Unlock()
	r := httptest.NewRequest(http.MethodPost, "/", &brokenChunk{})
	route := chi.NewRouteContext()
	route.URLParams.Add("id", u.ID)
	kind, id, _ := strings.Cut(upload.owner, ":")
	ctx := context.WithValue(r.Context(), ctxRequestInfo, &requestInfo{identity: &auth.Identity{Kind: kind, ID: id}})
	ctx = context.WithValue(ctx, chi.RouteCtxKey, route)
	r = r.WithContext(ctx)
	r.URL.RawQuery = "offset=0"
	w := httptest.NewRecorder()
	h.api.handleTransferChunk(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("broken chunk status: %d", w.Code)
	}
	var f os.FileInfo
	var err error
	f, err = os.Stat(upload.path)
	if err != nil || f.Size() != 0 || upload.Offset != 0 {
		t.Fatalf("partial bytes committed: %v %+v", err, upload)
	}
}
func TestChunkedTransferVerificationProducesAReceiptWithoutReplacingLiveData(t *testing.T) {
	source := newHarness(t)
	cookie := operatorCookie(source)
	source.do(request{method: http.MethodPost, path: "/api/v1/transfers/preparation", cookie: cookie}).mustStatus(t, http.StatusAccepted, "prepare source")
	archive := source.do(request{method: http.MethodPost, path: "/api/v1/transfers/export", cookie: cookie, body: map[string]string{"passphrase": "a long transfer phrase"}})
	archive.mustStatus(t, http.StatusOK, "export source")
	dest := newHarness(t)
	destCookie := operatorCookie(dest)
	u := startUpload(t, dest, destCookie, int64(len(archive.body)))
	path := "/api/v1/transfers/uploads/" + u.ID
	for offset := 0; offset < len(archive.body); {
		end := min(offset+1024, len(archive.body))
		dest.do(request{method: http.MethodPost, path: path + "/chunks?offset=" + strconv.Itoa(offset), cookie: destCookie, rawBody: string(archive.body[offset:end]), headers: map[string]string{"Content-Type": "application/octet-stream"}}).mustStatus(t, http.StatusOK, "upload ciphertext")
		offset = end
	}
	for i := 0; i < 2; i++ {
		dest.do(request{method: http.MethodPost, path: path + "/verify", cookie: destCookie, body: map[string]string{"passphrase": "a long transfer phrase"}}).mustStatus(t, http.StatusAccepted, "idempotent verification")
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		status := dest.do(request{method: http.MethodGet, path: path, cookie: destCookie})
		status.mustStatus(t, http.StatusOK, "verification status")
		status.into(t, &u)
		if u.State != "verifying" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("verification did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if u.State != "verified" || u.Backup == nil || u.Backup.Transfer == nil || !u.Backup.Transfer.Prepared {
		t.Fatalf("no verified receipt: %+v", u)
	}
	if dest.ctrl.Fenced().Fenced {
		t.Fatal("verification changed destination fence")
	}
	dest.api.transfers.mu.Lock()
	upload := dest.api.transfers.items[u.ID]
	dest.api.transfers.mu.Unlock()
	// Wait for worker cleanup, which follows the final receipt.
	for time.Now().Before(deadline) {
		if _, err := os.Stat(upload.path); os.IsNotExist(err) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("verified ciphertext was retained")
}

func TestTransferUploadsRequireOperatorAuthorityAndExpire(t *testing.T) {
	h := newHarness(t)
	user, _ := h.user("fleet-admin-upload", store.RoleAdmin)
	h.do(request{method: http.MethodPost, path: "/api/v1/transfers/uploads", cookie: h.session(user), body: map[string]int64{"size": 3}}).mustStatus(t, http.StatusForbidden, "fleet administrator cannot upload transfers")
	cookie := operatorCookie(h)
	u := startUpload(t, h, cookie, 3)
	h.api.transfers.mu.Lock()
	stored := h.api.transfers.items[u.ID]
	stored.expires = time.Now().Add(-time.Second)
	h.api.transfers.mu.Unlock()
	h.do(request{method: http.MethodGet, path: "/api/v1/transfers/uploads/" + u.ID, cookie: cookie}).mustStatus(t, http.StatusNotFound, "expired upload")
	startUpload(t, h, cookie, 3)
	if _, err := os.Stat(stored.path); !os.IsNotExist(err) {
		t.Fatalf("expired upload ciphertext retained: %v", err)
	}
}

func TestRejectedChunkedArchivesLeaveNoReceiptOrCiphertext(t *testing.T) {
	h := newHarness(t)
	cookie := operatorCookie(h)
	u := startUpload(t, h, cookie, 6)
	path := "/api/v1/transfers/uploads/" + u.ID
	h.do(request{method: http.MethodPost, path: path + "/chunks?offset=0", cookie: cookie, rawBody: "broken"}).mustStatus(t, http.StatusOK, "upload invalid archive")
	h.do(request{method: http.MethodPost, path: path + "/verify", cookie: cookie, body: map[string]string{"passphrase": "secret archive phrase"}}).mustStatus(t, http.StatusAccepted, "verify invalid archive")
	h.api.transfers.mu.Lock()
	stored := h.api.transfers.items[u.ID]
	h.api.transfers.mu.Unlock()
	select {
	case <-stored.done:
	case <-time.After(5 * time.Second):
		t.Fatal("invalid archive verification did not finish")
	}
	status := h.do(request{method: http.MethodGet, path: path, cookie: cookie})
	status.into(t, &u)
	if u.State != "failed" || u.Backup != nil || u.Error == "" || strings.Contains(u.Error, "secret archive phrase") {
		t.Fatalf("unsafe failed receipt: %+v", u)
	}
	if _, err := os.Stat(stored.path); !os.IsNotExist(err) {
		t.Fatalf("failed ciphertext retained: %v", err)
	}
	if h.ctrl.Fenced().Fenced {
		t.Fatal("failed verification changed live destination")
	}
}

type slowChunk struct {
	first bool
	done  bool
}

func (b *slowChunk) Read(p []byte) (int, error) {
	if b.done {
		return 0, io.EOF
	}
	if !b.first {
		b.first = true
		return copy(p, "abc"), nil
	}
	time.Sleep(60 * time.Millisecond)
	b.done = true
	return copy(p, "def"), nil
}
func TestTransferChunksCanOutliveTheOrdinaryRequestReadDeadline(t *testing.T) {
	h := newHarness(t)
	cookie := operatorCookie(h)
	u := startUpload(t, h, cookie, 6)
	server := httptest.NewUnstartedServer(h.api.Handler())
	server.Config.ReadTimeout = 10 * time.Millisecond
	server.Start()
	defer server.Close()
	r, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/transfers/uploads/"+u.ID+"/chunks?offset=0", &slowChunk{})
	if err != nil {
		t.Fatal(err)
	}
	r.ContentLength = 6
	r.AddCookie(&http.Cookie{Name: SessionCookie, Value: cookie})
	r.Header.Set("Origin", server.URL)
	response, err := server.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("slow bounded chunk: %d %s", response.StatusCode, body)
	}
}
