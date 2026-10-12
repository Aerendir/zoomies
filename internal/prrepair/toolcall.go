package prrepair

import (
	"bytes"
	"encoding/json"
	"strings"
)

// Call is a tool call a model made, as far as the repair needs to know it.
type Call struct {
	Name      string
	Arguments json.RawMessage
}

// ReplyFromCalls reads a model's tool calls as the reply it was asked for, and
// says whether it could.
//
// A repair offers no tools. Its replies are JSON objects the system prompt
// describes: {"read":["path"]} to see a file, {"windows":[...]} to see part of
// one, {"summary":...,"edits":[...]} to fix it, {"cannot_fix":"..."} to stop. A
// model trained to call functions takes those words for functions and calls
// `read` or `windows`, and on a repair that was one step from finished the
// answer was to give up. The call says exactly what the reply would have, so it
// is read as the reply. The result is only returned when the repair would have
// accepted it typed out, which is checked by decoding it as the repair does; the
// path allow-list, the bounds and the pinned head still apply to it afterwards,
// so this grants the model nothing it could not already ask for.
//
// What it cannot read is left for the caller to refuse as a tool the model was
// not offered: a call it only half understands is a guess at what was meant.
func ReplyFromCalls(calls []Call) (string, bool) {
	if len(calls) == 0 {
		return "", false
	}
	var reads []string
	var windows []ReadWindow
	seen := map[string]bool{}
	for _, c := range calls {
		var args any
		if err := json.Unmarshal(c.Arguments, &args); err != nil {
			return "", false
		}
		switch k := callKind(c.Name); k {
		case kindRead:
			paths, ok := pathsFrom(args)
			if !ok {
				return "", false
			}
			for _, p := range paths {
				if !seen[p] {
					seen[p] = true
					reads = append(reads, p)
				}
			}
		case kindWindows:
			found, ok := windowsFrom(args)
			if !ok {
				return "", false
			}
			windows = append(windows, found...)
		case kindFix, kindGiveUp:
			// A fix or a giving up ends the repair, so it stands alone: beside a
			// read it would be two replies, and the repair accepts one.
			if len(calls) != 1 {
				return "", false
			}
			if k == kindGiveUp {
				return giveUpReply(args)
			}
			return planReply(c.Arguments, args)
		default:
			return "", false
		}
	}
	out, err := json.Marshal(struct {
		Read    []string     `json:"read,omitempty"`
		Windows []ReadWindow `json:"windows,omitempty"`
	}{reads, windows})
	if err != nil {
		return "", false
	}
	return accepted(string(out))
}

type kind int

const (
	kindUnknown kind = iota
	kindRead
	kindWindows
	kindFix
	kindGiveUp
)

// callKind is what a tool name stands for. Some providers prefix a name with a
// namespace, and some models say read_file for read; both mean the reply.
func callKind(name string) kind {
	name = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(name)), "functions.")
	switch name {
	case "read", "read_file", "read_files":
		return kindRead
	case "windows", "read_window", "read_windows":
		return kindWindows
	case "edits", "files", "summary":
		return kindFix
	case "cannot_fix":
		return kindGiveUp
	}
	return kindUnknown
}

// pathsFrom finds the paths a read asks for in whichever shape the model wrote
// them: the reply's own {"read":[...]}, a single {"path":"..."}, a list of
// strings, or a list of objects with a path.
func pathsFrom(v any) ([]string, bool) {
	switch v := v.(type) {
	case string:
		return []string{v}, v != ""
	case []any:
		var out []string
		for _, item := range v {
			switch item := item.(type) {
			case string:
				if item == "" {
					return nil, false
				}
				out = append(out, item)
			case map[string]any:
				p, ok := item["path"].(string)
				if !ok || p == "" {
					return nil, false
				}
				out = append(out, p)
			default:
				return nil, false
			}
		}
		return out, len(out) > 0
	case map[string]any:
		for _, key := range []string{"read", "paths", "files", "path", "file"} {
			if inner, present := v[key]; present {
				return pathsFrom(inner)
			}
		}
	}
	return nil, false
}

// windowsFrom finds the windows a read asks for: the reply's own
// {"windows":[...]}, a list of windows, or one window written as the object.
// A window is decoded as the repair decodes it, so a line given as text, or a
// field the repair has no word for, is not read.
func windowsFrom(v any) ([]ReadWindow, bool) {
	var items []any
	switch v := v.(type) {
	case []any:
		items = v
	case map[string]any:
		if inner, ok := v["windows"]; ok {
			return windowsFrom(inner)
		}
		items = []any{v}
	default:
		return nil, false
	}
	out := make([]ReadWindow, 0, len(items))
	for _, item := range items {
		raw, err := json.Marshal(item)
		if err != nil {
			return nil, false
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		var w ReadWindow
		if err := dec.Decode(&w); err != nil || w.Path == "" {
			return nil, false
		}
		out = append(out, w)
	}
	return out, len(out) > 0
}

// giveUpReply reads a call that stops the repair: the reason, in the reply's own
// shape, or as a bare string, or under the name a model is likelier to give it.
func giveUpReply(args any) (string, bool) {
	var reason string
	switch v := args.(type) {
	case string:
		reason = v
	case map[string]any:
		for _, key := range []string{"cannot_fix", "reason", "message"} {
			if s, ok := v[key].(string); ok && s != "" {
				reason = s
				break
			}
		}
	}
	if reason == "" {
		return "", false
	}
	out, err := json.Marshal(struct {
		CannotFix string `json:"cannot_fix"`
	}{reason})
	if err != nil {
		return "", false
	}
	return accepted(string(out))
}

// planReply reads a call that proposes the fix. Its arguments have to be the
// reply itself, a summary with edits or files: a bare list of edits has no
// summary to publish, and one that is invented is not what the model said.
func planReply(raw json.RawMessage, args any) (string, bool) {
	if _, isObject := args.(map[string]any); !isObject {
		return "", false
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return "", false
	}
	p, err := decode(compact.String())
	if err != nil || strings.TrimSpace(p.Summary) == "" || len(p.Edits)+len(p.Files) == 0 || len(p.Read)+len(p.Windows) > 0 || p.CannotFix != "" {
		return "", false
	}
	return accepted(compact.String())
}

// accepted returns a reply only if the repair would decode it.
func accepted(reply string) (string, bool) {
	if _, err := decode(reply); err != nil {
		return "", false
	}
	return reply, true
}
