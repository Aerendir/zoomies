package wsl

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

// fakeSys is an in-memory host: files, environment and the commands it knows.
type fakeSys struct {
	goos  string
	env   map[string]string
	files map[string]string
	euid  int
	cmds  map[string]func(args []string) ([]byte, error)
	ran   []string
}

func newFake() *fakeSys {
	return &fakeSys{goos: "linux", env: map[string]string{"WSL_DISTRO_NAME": "Ubuntu"}, files: map[string]string{}, euid: 0,
		cmds: map[string]func([]string) ([]byte, error){}}
}

func (f *fakeSys) GOOS() string                       { return f.goos }
func (f *fakeSys) Getenv(k string) string             { return f.env[k] }
func (f *fakeSys) Geteuid() int                       { return f.euid }
func (f *fakeSys) MkdirAll(string, os.FileMode) error { return nil }
func (f *fakeSys) Exists(p string) bool               { _, ok := f.files[p]; return ok }
func (f *fakeSys) ReadFile(p string) ([]byte, error) {
	if s, ok := f.files[p]; ok {
		return []byte(s), nil
	}
	return nil, fs.ErrNotExist
}
func (f *fakeSys) WriteFile(p string, d []byte, _ os.FileMode) error {
	f.files[p] = string(d)
	return nil
}
func (f *fakeSys) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.ran = append(f.ran, name+" "+strings.Join(args, " "))
	if fn, ok := f.cmds[name]; ok {
		return fn(args)
	}
	return nil, errors.New("unexpected command " + name)
}

// wslHost is a fake with Windows interop and a scheduled-task store, the way a
// real distro looks once WSL, systemd and the agent unit are all there.
func wslHost() *fakeSys {
	f := newFake()
	f.files[SchtasksPath] = ""
	f.files["/etc/systemd/system/zoomies-agent.service"] = "[Service]\nUser=zoomies\n"
	f.cmds[CmdExePath] = func([]string) ([]byte, error) { return []byte("C:\\Users\\me\\AppData\\Local\r\n"), nil }
	f.cmds["wslpath"] = func(a []string) ([]byte, error) { return []byte("/mnt/c/Users/me/AppData/Local\n"), nil }
	f.cmds["loginctl"] = func(a []string) ([]byte, error) {
		f.files[lingerDir+"/"+a[1]] = ""
		return nil, nil
	}
	tasks := map[string]string{}
	f.cmds[SchtasksPath] = func(a []string) ([]byte, error) {
		name := a[2]
		switch a[0] {
		case "/Query":
			x, ok := tasks[name]
			if !ok {
				return nil, errors.New("ERROR: The system cannot find the file specified.")
			}
			return []byte(x), nil
		case "/Create":
			xml := a[4]
			for i, v := range a {
				if v == "MINUTE" && a[i-1] == "/SC" {
					xml += "<Interval>PT5M</Interval>"
				}
				if v == "ONSTART" {
					xml += "<BootTrigger>"
				}
			}
			tasks[name] = xml
			return nil, nil
		}
		return nil, errors.New("bad")
	}
	return f
}

func TestDetectFindsWSL2ByEnvironmentOrKernel(t *testing.T) {
	cases := []struct {
		name string
		mod  func(*fakeSys)
		want bool
	}{
		{"distro name set", func(*fakeSys) {}, true},
		{"kernel only (a systemd service has no WSL_DISTRO_NAME)", func(f *fakeSys) {
			f.env = nil
			f.files["/proc/version"] = "Linux version 6.18.40.1-microsoft-standard-WSL2 (root@x)"
		}, true},
		{"plain linux", func(f *fakeSys) { f.env = nil; f.files["/proc/version"] = "Linux version 6.8.0-generic" }, false},
		{"not linux", func(f *fakeSys) { f.goos = "windows" }, false},
	}
	for _, c := range cases {
		f := newFake()
		c.mod(f)
		if got := Detect(f); got != c.want {
			t.Errorf("%s: Detect = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestDistroFallsBackToTheWindowsPathOfRoot(t *testing.T) {
	f := newFake()
	f.env = nil
	f.cmds["wslpath"] = func([]string) ([]byte, error) { return []byte(`\\wsl.localhost\Ubuntu-24.04\` + "\n"), nil }
	if got := Distro(context.Background(), f); got != "Ubuntu-24.04" {
		t.Fatalf("Distro = %q", got)
	}
}

func TestBootCommandIsAddedOnceAndKeepsTheOperatorsOwn(t *testing.T) {
	cmd := BootCommand("/usr/local/bin/zoomies")
	out, changed := WithBootCommand("[automount]\nenabled=true\n", cmd)
	if !changed || !strings.Contains(out, "[boot]\ncommand="+cmd) || !strings.Contains(out, "[automount]") {
		t.Fatalf("first write: %q", out)
	}
	if again, changed := WithBootCommand(out, cmd); changed || again != out {
		t.Fatalf("second write changed the file: %q", again)
	}
	mine, _ := WithBootCommand("[boot]\nsystemd=true\ncommand=service cron start\n", cmd)
	if !strings.Contains(mine, "command=service cron start; "+cmd) || !strings.Contains(mine, "systemd=true") {
		t.Fatalf("an existing command was lost: %q", mine)
	}
	moved, changed := WithBootCommand(mine, BootCommand("/opt/zoomies"))
	if !changed || strings.Count(moved, bootMarker) != 2 && !strings.Contains(moved, "/opt/zoomies") {
		t.Fatalf("a moved binary was not followed: %q", moved)
	}
}

func TestEnsureSetsEverythingUpThenChangesNothing(t *testing.T) {
	f := wslHost()
	opts := Options{Binary: "/usr/local/bin/zoomies"}
	first := Ensure(context.Background(), f, opts)
	if !first.Changed() || len(first.Warnings()) != 0 {
		t.Fatalf("first run: %+v", first)
	}
	if !strings.Contains(f.files[ConfPath], "command=/usr/local/bin/zoomies agent --wsl-ensure") {
		t.Fatalf("wsl.conf: %q", f.files[ConfPath])
	}
	if _, ok := f.files[lingerDir+"/zoomies"]; !ok {
		t.Fatal("linger was not enabled for the unit's user")
	}
	second := Ensure(context.Background(), f, opts)
	if second.Changed() {
		t.Fatalf("a second run changed something: %+v", second.Steps)
	}
}

func TestEnsureRepairsADriftedTask(t *testing.T) {
	f := wslHost()
	opts := Options{Binary: "/z"}
	Ensure(context.Background(), f, opts)
	// Someone retargets the five-minute task: the next run puts it back.
	f.cmds[SchtasksPath] = func(a []string) ([]byte, error) {
		if a[0] == "/Query" && a[2] == taskEvery5 {
			return []byte("<Interval>PT1H</Interval>"), nil
		}
		return wslHost().cmds[SchtasksPath](a)
	}
	rep := Ensure(context.Background(), f, opts)
	var repaired bool
	for _, s := range rep.Steps {
		if s.Name == "task "+taskEvery5 && s.Status == StatusChanged {
			repaired = true
		}
	}
	if !repaired {
		t.Fatalf("drifted task was not repaired: %+v", rep.Steps)
	}
}

func TestEnsureWarnsInsteadOfAskingWhenWindowsWantsElevation(t *testing.T) {
	f := wslHost()
	f.cmds[SchtasksPath] = func(a []string) ([]byte, error) {
		if a[0] == "/Query" {
			return nil, errors.New("not found")
		}
		return []byte("ERROR: Access is denied."), errors.New("exit status 1")
	}
	rep := Ensure(context.Background(), f, Options{Binary: "/z"})
	if len(rep.Warnings()) < 2 {
		t.Fatalf("expected the tasks to warn: %+v", rep.Steps)
	}
	for _, c := range f.ran {
		for _, banned := range []string{"/RP", "/RU", "/P "} {
			if strings.Contains(c, banned) {
				t.Fatalf("a credential flag was passed: %s", c)
			}
		}
	}
}

func TestEnsureNeedsRootForWslConfButStillDoesTheRest(t *testing.T) {
	f := wslHost()
	f.euid = 1000
	rep := Ensure(context.Background(), f, Options{Binary: "/z"})
	if _, wrote := f.files[ConfPath]; wrote {
		t.Fatal("wsl.conf was written without root")
	}
	if rep.Warnings()[0].Name != "wsl.conf boot.command" {
		t.Fatalf("warnings: %+v", rep.Warnings())
	}
	if _, ok := f.files[SchtasksPath]; !ok || len(f.ran) == 0 {
		t.Fatal("the Windows tasks were skipped because of an unrelated warning")
	}
}

func TestEnsureOutsideWSLDoesNothing(t *testing.T) {
	f := newFake()
	f.env = nil
	f.files["/proc/version"] = "Linux version 6.8.0"
	rep := Ensure(context.Background(), f, Options{Binary: "/z"})
	if rep.Changed() || len(f.ran) != 0 || len(f.files) != 1 {
		t.Fatalf("touched a non-WSL host: %+v ran=%v", rep, f.ran)
	}
}

func TestDecodeConsoleReadsUTF16(t *testing.T) {
	in := []byte{0xFF, 0xFE, '<', 0, 'B', 0}
	if got := decodeConsole(in); got != "<B" {
		t.Fatalf("got %q", got)
	}
}

func TestStartScriptQuotesTheDistroName(t *testing.T) {
	if s := startScript("O'Brien"); !strings.Contains(s, "$distro = 'O''Brien'") {
		t.Fatalf("unquoted: %s", s)
	}
}

func TestKeepaliveStartsOnceAdoptsAfterRestartAndRestartsWhenItDies(t *testing.T) {
	files := map[string]string{}
	alive := map[int]bool{}
	var started []int
	next := 100
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	k := &Keepalive{
		Distro: "Ubuntu", WSLExe: WSLExePath, PidFile: "/pid", Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Start: func(_ context.Context, argv []string) (int, error) {
			if argv[0] != WSLExePath || argv[len(argv)-1] != "infinity" {
				t.Errorf("argv = %v", argv)
			}
			next++
			alive[next] = true
			started = append(started, next)
			return next, nil
		},
		Alive: func(pid int) bool { return alive[pid] },
		Read: func(p string) ([]byte, error) {
			if s, ok := files[p]; ok {
				return []byte(s), nil
			}
			return nil, fs.ErrNotExist
		},
		Write: func(p string, d []byte, _ os.FileMode) error { files[p] = string(d); return nil },
	}
	polls := 0
	k.Sleep = func(ctx context.Context, _ time.Duration) bool {
		polls++
		switch polls {
		case 2:
			alive[started[0]] = false // the session dies: expect a second one
		case 5:
			cancel()
		}
		return ctx.Err() == nil
	}
	k.Run(ctx)
	if len(started) != 2 {
		t.Fatalf("started %v, want two sessions", started)
	}
	if !alive[started[1]] {
		t.Fatal("stopping the agent ended the keepalive; the distro would go down under running jobs")
	}

	// A restarted agent adopts the one still alive instead of starting a third.
	ctx2, cancel2 := context.WithCancel(context.Background())
	k.Sleep = func(context.Context, time.Duration) bool { cancel2(); return false }
	k.Run(ctx2)
	if len(started) != 2 {
		t.Fatalf("a restart started another keepalive: %v", started)
	}
}
