package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eyupio/zoomies/internal/backup"
	"github.com/eyupio/zoomies/internal/store"
)

const transferChunkBytes = 8 << 20
const transferUploadTTL = 24 * time.Hour

// Upload state is transport state, not fleet state. A restart discards it;
// only a verified backup survives, and applying it remains a separate act.
type transferUploads struct {
	mu      sync.Mutex
	items   map[string]*transferUpload
	stopped bool
}
type transferUpload struct {
	mu         sync.Mutex
	ID         string      `json:"id"`
	Size       int64       `json:"size"`
	Offset     int64       `json:"offset"`
	ChunkBytes int64       `json:"chunk_bytes"`
	State      string      `json:"state"`
	Backup     *backupView `json:"backup,omitempty"`
	Error      string      `json:"error,omitempty"`
	owner      string
	path       string
	expires    time.Time
	cancel     context.CancelFunc
	done       chan struct{}
}

func uploadOwner(r *http.Request) string {
	id := Identity(r.Context())
	return id.Kind + ":" + id.ID
}
func (s *Server) transferUpload(r *http.Request) *transferUpload {
	s.transfers.mu.Lock()
	defer s.transfers.mu.Unlock()
	u := s.transfers.items[chiURLParam(r, "id")]
	if s.transfers.stopped || u == nil || u.owner != uploadOwner(r) || time.Now().After(u.expires) {
		return nil
	}
	return u
}
func (s *Server) handleCreateTransferUpload(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Size int64 `json:"size"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if !decode(w, r, &body) {
		return
	}
	if body.Size <= 0 || body.Size > maxBackupUploadBytes {
		badRequest(w, "choose an archive between 1 byte and 8 GiB")
		return
	}
	s.transfers.mu.Lock()
	defer s.transfers.mu.Unlock()
	if s.transfers.stopped {
		conflict(w, "the controller is stopping")
		return
	}
	if s.transfers.items == nil {
		s.transfers.items = make(map[string]*transferUpload)
	}
	root := filepath.Join(s.ctrl.BackupDir(), ".transfer-uploads")
	if err := os.MkdirAll(root, 0700); err != nil {
		s.internal(w, r, "creating the upload directory", err)
		return
	}
	// Reap ciphertext left by a previous process, as well as expired receipts.
	files, _ := os.ReadDir(root)
	for _, f := range files {
		if info, err := f.Info(); err == nil && time.Since(info.ModTime()) > transferUploadTTL {
			_ = os.Remove(filepath.Join(root, f.Name()))
		}
	}
	active, owned := 0, 0
	for id, u := range s.transfers.items {
		u.mu.Lock()
		if time.Now().After(u.expires) {
			if u.cancel != nil {
				u.cancel()
			}
			_ = os.Remove(u.path)
			delete(s.transfers.items, id)
		} else if u.State == "uploading" || u.State == "verifying" {
			active++
			if u.owner == uploadOwner(r) {
				owned++
			}
		}
		u.mu.Unlock()
	}
	if active >= 16 || owned >= 2 || len(s.transfers.items) >= 128 {
		conflict(w, "too many uploads are waiting; finish or cancel an existing upload first")
		return
	}
	f, err := os.CreateTemp(root, "archive-*")
	if err != nil {
		s.internal(w, r, "creating an upload", err)
		return
	}
	_ = f.Close()
	u := &transferUpload{ID: store.NewID("upload"), Size: body.Size, ChunkBytes: transferChunkBytes, State: "uploading", owner: uploadOwner(r), path: f.Name(), expires: time.Now().Add(transferUploadTTL)}
	s.transfers.items[u.ID] = u
	// Cleanup must also happen on an otherwise idle controller.
	time.AfterFunc(transferUploadTTL, func() {
		s.transfers.mu.Lock()
		defer s.transfers.mu.Unlock()
		u.mu.Lock()
		defer u.mu.Unlock()
		if u.cancel != nil {
			u.cancel()
		}
		_ = os.Remove(u.path)
		delete(s.transfers.items, u.ID)
	})
	writeCreated(w, r, u.ID, u)
}
func (s *Server) handleGetTransferUpload(w http.ResponseWriter, r *http.Request) {
	u := s.transferUpload(r)
	if u == nil {
		notFound(w, "upload")
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	writeJSON(w, http.StatusOK, u)
}
func (s *Server) handleCancelTransferUpload(w http.ResponseWriter, r *http.Request) {
	u := s.transferUpload(r)
	if u == nil {
		notFound(w, "upload")
		return
	}
	u.mu.Lock()
	if u.State == "verifying" {
		u.mu.Unlock()
		conflict(w, "archive verification is already running")
		return
	}
	_ = os.Remove(u.path)
	u.State = "cancelled"
	u.mu.Unlock()
	s.transfers.mu.Lock()
	delete(s.transfers.items, u.ID)
	s.transfers.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTransferChunk(w http.ResponseWriter, r *http.Request) {
	u := s.transferUpload(r)
	if u == nil {
		notFound(w, "upload")
		return
	}
	offset, err := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
	if err != nil || offset < 0 {
		badRequest(w, "send the chunk's byte offset")
		return
	}
	// A slow uplink can take longer than the server's ordinary request
	// deadline. Keep each bounded chunk finite without imposing that short cap.
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(2 * time.Minute))
	// Buffer a bounded chunk before committing any bytes: an interrupted body
	// cannot turn a retry into a corrupted archive or advance its offset.
	raw, err := io.ReadAll(io.LimitReader(r.Body, transferChunkBytes+1))
	if err != nil {
		badRequest(w, "the chunk did not arrive completely; retry it")
		return
	}
	if len(raw) == 0 || len(raw) > transferChunkBytes {
		payloadTooLarge(w, "send chunks of 1 byte to 8 MiB")
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.State != "uploading" || offset > u.Offset || offset+int64(len(raw)) > u.Size {
		conflict(w, "that chunk does not follow this upload's offset")
		return
	}
	f, err := os.OpenFile(u.path, os.O_RDWR, 0600)
	if err != nil {
		s.internal(w, r, "opening the upload", err)
		return
	}
	defer f.Close()
	if offset < u.Offset {
		if offset+int64(len(raw)) > u.Offset {
			conflict(w, "a replay must contain only bytes already uploaded")
			return
		}
		old := make([]byte, len(raw))
		if _, err := f.ReadAt(old, offset); err != nil || !bytes.Equal(old, raw) {
			conflict(w, "that chunk differs from the bytes already uploaded")
			return
		}
	} else {
		if _, err := f.WriteAt(raw, offset); err != nil {
			_ = f.Truncate(u.Offset)
			s.internal(w, r, "writing the upload", err)
			return
		}
		if err := f.Sync(); err != nil {
			_ = f.Truncate(u.Offset)
			s.internal(w, r, "saving the upload", err)
			return
		}
		u.Offset += int64(len(raw))
	}
	writeJSON(w, http.StatusOK, u)
}
func (s *Server) handleVerifyTransferUpload(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Passphrase string `json:"passphrase"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if !decode(w, r, &body) {
		return
	}
	if len(body.Passphrase) == 0 || len(body.Passphrase) > 4096 {
		badRequest(w, "send the archive passphrase, up to 4096 bytes")
		return
	}
	u := s.transferUpload(r)
	if u == nil {
		notFound(w, "upload")
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.State != "uploading" {
		if u.State == "cancelled" {
			conflict(w, "this upload was cancelled")
			return
		}
		writeJSON(w, http.StatusAccepted, u)
		return
	}
	if u.Offset != u.Size {
		conflict(w, "finish uploading the archive before verifying it")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	u.cancel = cancel
	u.State = "verifying"
	u.done = make(chan struct{})
	id := *Identity(r.Context())
	cfg := s.cfg()
	go func() {
		defer close(u.done)
		defer cancel()
		defer os.Remove(u.path)
		f, err := os.Open(u.path)
		var entry *backup.Entry
		if err == nil {
			entry, err = backup.ReadTransfer(ctx, cfg, s.ctrl.BackupDir(), f, body.Passphrase)
			_ = f.Close()
		}
		u.mu.Lock()
		defer u.mu.Unlock()
		if err != nil {
			u.State = "failed"
			if errors.Is(err, backup.ErrWrongPassphrase) {
				u.Error = "The archive did not open. Check its passphrase."
			} else {
				u.Error = "That file is not an instance archive this controller can use: " + strings.TrimPrefix(err.Error(), "backup: ")
			}
			// Do not retain or log the supplied passphrase in an error.
			u.Error = strings.ReplaceAll(u.Error, body.Passphrase, "[redacted]")
		} else {
			view := s.backupView(*entry, locationBackups)
			u.Backup = &view
			u.State = "verified"
			s.auth.Auditor().Act(ctx, &id, "backup.upload", "backup", entry.ID, map[string]any{"encrypted": true, "bytes": entry.Bytes, "taken_at": entry.TakenAt})
		}
	}()
	writeJSON(w, http.StatusAccepted, u)
}

// Stop verification before the controller closes its database. A browser
// disconnect does not stop the job, but shutting down the controller does.
func (s *Server) stopTransferUploads() {
	s.transfers.mu.Lock()
	s.transfers.stopped = true
	var done []chan struct{}
	for _, u := range s.transfers.items {
		u.mu.Lock()
		if u.cancel != nil {
			u.cancel()
		}
		if u.done != nil {
			done = append(done, u.done)
		}
		if u.State == "uploading" {
			u.State = "cancelled"
			_ = os.Remove(u.path)
		}
		u.mu.Unlock()
	}
	s.transfers.mu.Unlock()
	for _, ch := range done {
		<-ch
	}
}
