package wsl

import "strings"

// bootMarker is what identifies a boot command as ours, so that a second run
// finds it and changes nothing.
const bootMarker = "agent --wsl-ensure"

// BootCommand is the line written for boot.command: it runs as root when the
// distro starts, does its idempotent work and exits, so it never delays boot.
func BootCommand(binary string) string { return binary + " " + bootMarker }

// WithBootCommand returns conf with boot.command set to cmd under [boot] and
// whether that changed anything. A boot command the operator already had is
// kept and ours is chained after it, because wsl.conf has room for exactly one.
// Everything else in the file, comments included, is left as it was.
func WithBootCommand(conf, cmd string) (string, bool) {
	lines := strings.Split(conf, "\n")
	boot, end := -1, len(lines)
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if !strings.HasPrefix(t, "[") {
			continue
		}
		if boot >= 0 {
			end = i
			break
		}
		if strings.EqualFold(strings.Trim(t, "[] "), "boot") {
			boot = i
		}
	}
	if boot < 0 {
		out := strings.TrimRight(conf, "\n")
		if out != "" {
			out += "\n\n"
		}
		return out + "[boot]\ncommand=" + cmd + "\n", true
	}
	for i := boot + 1; i < end; i++ {
		k, v, ok := strings.Cut(strings.TrimSpace(lines[i]), "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(k), "command") {
			continue
		}
		existing := strings.Trim(strings.TrimSpace(v), `"`)
		switch {
		case strings.Contains(existing, bootMarker) && strings.Contains(existing, cmd):
			return conf, false
		case strings.Contains(existing, bootMarker):
			// Ours, from another binary path (the binary moved): replace it.
			lines[i] = "command=" + cmd
		case existing == "":
			lines[i] = "command=" + cmd
		default:
			lines[i] = "command=" + existing + "; " + cmd
		}
		return strings.Join(lines, "\n"), true
	}
	lines = append(lines[:boot+1], append([]string{"command=" + cmd}, lines[boot+1:]...)...)
	return strings.Join(lines, "\n"), true
}

// HasSystemd reports whether conf asks for systemd, which linger and the
// agent's unit both depend on.
func HasSystemd(conf string) bool {
	section := ""
	for _, line := range strings.Split(conf, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "["):
			section = strings.ToLower(strings.Trim(line, "[] "))
		case section == "boot":
			if k, v, ok := strings.Cut(line, "="); ok && strings.EqualFold(strings.TrimSpace(k), "systemd") {
				return strings.EqualFold(strings.TrimSpace(v), "true")
			}
		}
	}
	return false
}
