package backend

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

// The exec calls are how the agent asks a runner's own container a question
// the host cannot answer from outside: how full a folder it mounted in memory
// is. They run a command to its end and hand back what it printed and how it
// ended, and they have to keep two things straight that a plain log reader
// does not: which stream a line came on, and what the daemon says when the
// command could not be started at all.
func TestACommandRunInAContainerReturnsWhatItPrintedAndHowItEnded(t *testing.T) {
	var created struct {
		AttachStdin  bool
		AttachStdout bool
		AttachStderr bool
		Tty          bool
		Cmd          []string
		Env          []string
	}
	var started struct {
		Detach bool
		Tty    bool
	}
	f := newFakeEngine(t, map[string]http.HandlerFunc{
		"POST " + v + "/containers/c1/exec": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&created)
			writeJSON(w, http.StatusCreated, map[string]string{"Id": "e1"})
		},
		"POST " + v + "/exec/e1/start": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&started)
			w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
			_, _ = w.Write(append(frame(1, "out\n"), frame(2, "warning\n")...))
		},
		"GET " + v + "/exec/e1/json": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]any{"Running": false, "ExitCode": 3})
		},
	})

	got, err := f.client(t).ContainerExec(context.Background(), "c1", []string{"df", "-P"}, []string{"LC_ALL=C"})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if got.Stdout != "out\n" || got.Stderr != "warning\n" || got.ExitCode != 3 {
		t.Fatalf("result = %+v, want the two streams kept apart and the exit code 3", got)
	}
	if created.AttachStdin || !created.AttachStdout || !created.AttachStderr || created.Tty {
		t.Fatalf("create asked for %+v: output on both streams, no input, no terminal", created)
	}
	if strings.Join(created.Cmd, " ") != "df -P" || strings.Join(created.Env, " ") != "LC_ALL=C" {
		t.Fatalf("create sent cmd %v env %v", created.Cmd, created.Env)
	}
	if started.Detach || started.Tty {
		t.Fatalf("start asked for %+v; it has to stay attached to read the output, without a terminal", started)
	}
}

// The stream ends when the command's output does, which can be a moment before
// the daemon has recorded that the command exited. Read at once, the exit code
// is the zero of a command still running, and a df that failed would be read
// as one that worked.
func TestTheExitCodeIsReadOnceTheDaemonSaysTheCommandHasEnded(t *testing.T) {
	var polls atomic.Int32
	f := newFakeEngine(t, map[string]http.HandlerFunc{
		"POST " + v + "/containers/c1/exec": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusCreated, map[string]string{"Id": "e1"})
		},
		"POST " + v + "/exec/e1/start": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(frame(1, "x\n"))
		},
		"GET " + v + "/exec/e1/json": func(w http.ResponseWriter, r *http.Request) {
			if polls.Add(1) < 3 {
				writeJSON(w, http.StatusOK, map[string]any{"Running": true, "ExitCode": 0})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Running": false, "ExitCode": 127})
		},
	})
	got, err := f.client(t).ContainerExec(context.Background(), "c1", []string{"df"}, nil)
	if err != nil || got.ExitCode != 127 {
		t.Fatalf("result = %+v, err = %v; want the exit code of the finished command, 127", got, err)
	}
}

// When the command cannot be started the daemon has already taken the
// connection over, so it cannot send an error status. It writes the reason as
// plain text on the stream instead, which is no frame at all, and the exit code
// afterwards is the shell's 127 for a command that is not there.
func TestADaemonThatCouldNotStartTheCommandIsReadAsText(t *testing.T) {
	const reason = `OCI runtime exec failed: exec failed: unable to start container process: exec: "df": executable file not found in $PATH: unknown`
	f := newFakeEngine(t, map[string]http.HandlerFunc{
		"POST " + v + "/containers/c1/exec": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusCreated, map[string]string{"Id": "e1"})
		},
		"POST " + v + "/exec/e1/start": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(reason + "\r\n"))
		},
		"GET " + v + "/exec/e1/json": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]any{"Running": false, "ExitCode": 127})
		},
	})
	got, err := f.client(t).ContainerExec(context.Background(), "c1", []string{"df"}, nil)
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if got.Stdout != "" || !strings.Contains(got.Stderr, "executable file not found") || got.ExitCode != 127 {
		t.Fatalf("result = %+v, want the daemon's sentence kept as the error text and no output", got)
	}
}

// Docker itself reports a command it could not start as a message on the
// stdout stream, in a frame like any other output, and the exit code afterwards
// is the one that says the command never ran. Caught against a real engine: the
// text is not outside the framing, as an older reading of the protocol had it.
func TestADockerThatCouldNotStartTheCommandSaysSoOnStdoutAndByItsExitCode(t *testing.T) {
	const reason = `OCI runtime exec failed: exec failed: unable to start container process: exec: "df": executable file not found in $PATH`
	f := newFakeEngine(t, map[string]http.HandlerFunc{
		"POST " + v + "/containers/c1/exec": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusCreated, map[string]string{"Id": "e1"})
		},
		"POST " + v + "/exec/e1/start": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(frame(1, reason+"\r\n"))
		},
		"GET " + v + "/exec/e1/json": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]any{"Running": false, "ExitCode": 127})
		},
	})
	got := measureScratch(context.Background(), f.client(t), "c1", []string{"/tmp"})
	if got.unmeasured != ScratchNoDF || len(got.use) != 0 {
		t.Fatalf("reading = %+v, want the image named as having no df", got)
	}
}

// A real daemon hijacks the connection for the output: it answers 200 with no
// length and no chunking, writes the stream, and closes the connection when the
// command's output ends. The client has to read that to the end of the
// connection, which a handler that merely writes a body does not prove.
func TestAnExecStreamTheDaemonHijacksIsReadToTheEndOfTheConnection(t *testing.T) {
	var upgrade string
	f := newFakeEngine(t, map[string]http.HandlerFunc{
		"POST " + v + "/containers/c1/exec": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusCreated, map[string]string{"Id": "e1"})
		},
		"POST " + v + "/exec/e1/start": func(w http.ResponseWriter, r *http.Request) {
			upgrade = r.Header.Get("Upgrade")
			conn, rw, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Errorf("hijack: %v", err)
				return
			}
			defer conn.Close()
			_, _ = rw.WriteString("HTTP/1.1 200 OK\r\nContent-Type: application/vnd.docker.raw-stream\r\n\r\n")
			_, _ = rw.Write(frame(1, "first\n"))
			_, _ = rw.Write(frame(1, "second\n"))
			_ = rw.Flush()
		},
		"GET " + v + "/exec/e1/json": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]any{"Running": false, "ExitCode": 0})
		},
	})
	got, err := f.client(t).ContainerExec(context.Background(), "c1", []string{"df"}, nil)
	if err != nil || got.Stdout != "first\nsecond\n" {
		t.Fatalf("result = %+v, err = %v; want everything the daemon wrote before it closed the connection", got, err)
	}
	if upgrade != "" {
		t.Fatalf("the start asked to upgrade the connection (%q); the client reads a plain response", upgrade)
	}
}

// A command that never stops printing must not fill the agent's memory: the
// output is cut at a fixed size, and what was read before the cut is kept.
func TestACommandsOutputIsBoundedNotUnlimited(t *testing.T) {
	chunk := strings.Repeat("x", 1<<10)
	f := newFakeEngine(t, map[string]http.HandlerFunc{
		"POST " + v + "/containers/c1/exec": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusCreated, map[string]string{"Id": "e1"})
		},
		"POST " + v + "/exec/e1/start": func(w http.ResponseWriter, r *http.Request) {
			for range 4 * execOutputLimit / len(chunk) {
				if _, err := w.Write(frame(1, chunk)); err != nil {
					return
				}
			}
		},
		"GET " + v + "/exec/e1/json": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]any{"Running": false, "ExitCode": 0})
		},
	})
	got, err := f.client(t).ContainerExec(context.Background(), "c1", []string{"yes"}, nil)
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if n := len(got.Stdout); n == 0 || n > execOutputLimit {
		t.Fatalf("kept %d bytes of output, want some and no more than %d", n, execOutputLimit)
	}
}

// What the daemon says when it will not run a command is the caller's to act
// on, so it arrives as the daemon's own error with its status, and a container
// that has gone is the usual not-found.
func TestAnExecTheDaemonRefusesCarriesItsStatus(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		gone   bool
	}{
		{"forbidden by a proxy or plugin", http.StatusForbidden, false},
		{"no such container", http.StatusNotFound, true},
		{"container not running", http.StatusConflict, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeEngine(t, map[string]http.HandlerFunc{
				"POST " + v + "/containers/c1/exec": func(w http.ResponseWriter, r *http.Request) {
					writeJSON(w, tc.status, map[string]string{"message": "no"})
				},
			})
			_, err := f.client(t).ContainerExec(context.Background(), "c1", []string{"df"}, nil)
			if StatusCode(err) != tc.status || errors.Is(err, ErrNotFound) != tc.gone {
				t.Fatalf("err = %v (status %d), want status %d", err, StatusCode(err), tc.status)
			}
		})
	}
}
