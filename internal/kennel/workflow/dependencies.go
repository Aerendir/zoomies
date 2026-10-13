package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

const DependencyRows = 100
const maxDependencyDeclarations = maxNodes / 2

// These are root-file declaration facts, never resolved dependencies. Raw names,
// paths, inputs and source text must not outlive the parser.
type DependencyInventory struct {
	Version      string                `json:"version"`
	Basis        string                `json:"basis"`
	SourceSHA256 string                `json:"source_sha256"`
	Coverage     string                `json:"coverage"`
	Counts       *DependencyCounts     `json:"counts"`
	RowsOmitted  bool                  `json:"rows_omitted"`
	References   []DependencyReference `json:"references"`
}
type DependencyCounts struct {
	Declarations int `json:"declarations"`
	CommitPins   int `json:"commit_pins"`
	DigestPins   int `json:"digest_pins"`
	Floating     int `json:"floating"`
	Local        int `json:"local"`
	Expressions  int `json:"expressions"`
	Unknown      int `json:"unknown"`
}
type DependencyReference struct {
	JobIndex        int     `json:"job_index"`
	StepIndex       int     `json:"step_index"`
	Line            int     `json:"line"`
	Declaration     string  `json:"declaration"`
	Kind            string  `json:"kind"`
	Pin             string  `json:"pin"`
	ReferenceSHA256 string  `json:"reference_sha256"`
	Revision        *string `json:"revision"`
}

var dependencyName = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
var dependencyImage = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/:-]*$`)

// InspectDependencies enumerates literal root uses positions only. Unsupported
// shapes discard partial facts; expressions and local paths stay unresolved.
func InspectDependencies(data []byte) *DependencyInventory {
	if len(data) > MaxBytes {
		return nil
	}
	digest := sha256.Sum256(data)
	unknown := &DependencyInventory{Version: "workflow-dependencies-v1", Basis: "root_uses_declarations", SourceSHA256: hex.EncodeToString(digest[:]), Coverage: "unsupported", References: []DependencyReference{}}
	root, err := parseRoot(data)
	if err != nil {
		return unknown
	}
	jobs := field(root, "jobs")
	if jobs == nil || jobs.Kind != yaml.MappingNode || len(jobs.Content) == 0 {
		return unknown
	}
	out := *unknown
	out.Coverage = "enumerated"
	out.Counts = &DependencyCounts{}
	add := func(node *yaml.Node, job, step int, declaration string) bool {
		if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
			return false
		}
		row := classifyDependency(node.Value)
		if declaration == "job_workflow" {
			invalid := row.Kind == "docker"
			if row.Kind == "remote" {
				name, _, _ := strings.Cut(node.Value, "@")
				parts := strings.Split(name, "/")
				invalid = len(parts) != 5 || parts[2] != ".github" || parts[3] != "workflows" || !(strings.HasSuffix(parts[4], ".yml") || strings.HasSuffix(parts[4], ".yaml"))
			}
			if invalid {
				row.Kind = "unknown"
				row.Pin = "unresolved"
				row.Revision = nil
			}
		}
		row.JobIndex = job
		row.StepIndex = step
		row.Line = node.Line
		row.Declaration = declaration
		out.Counts.Declarations++
		switch dependencyClass(row) {
		case "commit":
			out.Counts.CommitPins++
		case "digest":
			out.Counts.DigestPins++
		case "floating":
			out.Counts.Floating++
		case "local":
			out.Counts.Local++
		case "expression":
			out.Counts.Expressions++
		default:
			out.Counts.Unknown++
		}
		if len(out.References) < DependencyRows {
			out.References = append(out.References, row)
		} else {
			out.RowsOmitted = true
		}
		return true
	}
	for i := 0; i < len(jobs.Content); i += 2 {
		job := jobs.Content[i+1]
		if job.Kind != yaml.MappingNode {
			return unknown
		}
		uses, steps := field(job, "uses"), field(job, "steps")
		if uses != nil {
			if steps != nil || !add(uses, i/2, -1, "job_workflow") {
				return unknown
			}
			continue
		}
		if steps == nil || steps.Kind != yaml.SequenceNode {
			return unknown
		}
		for j, step := range steps.Content {
			if step.Kind != yaml.MappingNode {
				return unknown
			}
			if uses := field(step, "uses"); uses != nil {
				if field(step, "run") != nil || !add(uses, i/2, j, "step_action") {
					return unknown
				}
			}
		}
	}
	return &out
}
func classifyDependency(value string) DependencyReference {
	digest := sha256.Sum256([]byte(value))
	row := DependencyReference{Kind: "unknown", Pin: "unresolved", ReferenceSHA256: hex.EncodeToString(digest[:])}
	if strings.Contains(value, "${{") {
		row.Kind = "expression"
		return row
	}
	if value == "" || len(value) > 2048 {
		return row
	}
	for _, c := range value {
		if unicode.IsSpace(c) || unicode.IsControl(c) {
			return row
		}
	}
	if strings.HasPrefix(value, "./") {
		row.Kind = "local"
		return row
	}
	if strings.HasPrefix(value, "docker://") {
		image := strings.TrimPrefix(value, "docker://")
		name, ref, has := strings.Cut(image, "@")
		if strings.Contains(name, "://") || !dependencyImage.MatchString(name) {
			return row
		}
		if !has {
			row.Kind = "docker"
			row.Pin = "floating"
			return row
		}
		if strings.HasPrefix(ref, "sha256:") && dependencyHash(strings.TrimPrefix(ref, "sha256:"), 64) {
			row.Kind = "docker"
			row.Pin = "digest"
			v := strings.ToLower(ref)
			row.Revision = &v
		}
		return row
	}
	name, ref, has := strings.Cut(value, "@")
	if !has || ref == "" || strings.Contains(ref, "@") {
		return row
	}
	parts := strings.Split(name, "/")
	if len(parts) < 2 {
		return row
	}
	for _, p := range parts {
		if p == "." || p == ".." || !dependencyName.MatchString(p) {
			return row
		}
	}
	row.Kind = "remote"
	row.Pin = "floating"
	if commitSHA.MatchString(ref) {
		row.Pin = "commit"
		v := strings.ToLower(ref)
		row.Revision = &v
	}
	return row
}
func dependencyClass(r DependencyReference) string {
	if r.Kind == "local" || r.Kind == "expression" {
		return r.Kind
	}
	if r.Pin == "commit" || r.Pin == "digest" || r.Pin == "floating" {
		return r.Pin
	}
	return "unknown"
}
func dependencyHash(s string, n int) bool {
	_, err := hex.DecodeString(s)
	return len(s) == n && err == nil
}

// ValidDependencyInventory checks retained metadata, not discarded source bytes.
func ValidDependencyInventory(e *DependencyInventory, digest string) bool {
	if e == nil {
		return true
	}
	if e.Version != "workflow-dependencies-v1" || e.Basis != "root_uses_declarations" || e.SourceSHA256 != digest || !dependencyHash(digest, 64) || e.References == nil || len(e.References) > DependencyRows {
		return false
	}
	if e.Coverage == "unsupported" {
		return e.Counts == nil && !e.RowsOmitted && len(e.References) == 0
	}
	c := e.Counts
	if e.Coverage != "enumerated" || c == nil || c.Declarations < 0 || c.Declarations > maxDependencyDeclarations || c.Declarations < len(e.References) || e.RowsOmitted != (c.Declarations > len(e.References)) {
		return false
	}
	total := 0
	counts := map[string]int{"commit": c.CommitPins, "digest": c.DigestPins, "floating": c.Floating, "local": c.Local, "expression": c.Expressions, "unknown": c.Unknown}
	for _, n := range counts {
		if n < 0 || n > c.Declarations {
			return false
		}
		total += n
	}
	if total != c.Declarations {
		return false
	}
	seen := map[string]bool{}
	shown := map[string]int{}
	for _, r := range e.References {
		key := strconv.Itoa(r.JobIndex) + ":" + strconv.Itoa(r.StepIndex)
		if r.JobIndex < 0 || r.JobIndex > maxDependencyDeclarations || r.StepIndex < -1 || r.StepIndex > maxNodes || r.Line < 1 || r.Line > MaxBytes || seen[key] || !dependencyHash(r.ReferenceSHA256, 64) || r.ReferenceSHA256 != strings.ToLower(r.ReferenceSHA256) {
			return false
		}
		seen[key] = true
		if r.Declaration == "job_workflow" {
			if r.StepIndex != -1 {
				return false
			}
		} else if r.Declaration != "step_action" || r.StepIndex < 0 {
			return false
		}
		switch r.Kind {
		case "remote":
			if r.Pin != "commit" && r.Pin != "floating" {
				return false
			}
		case "docker":
			if r.Declaration != "step_action" || r.Pin != "digest" && r.Pin != "floating" {
				return false
			}
		case "local", "expression", "unknown":
			if r.Pin != "unresolved" {
				return false
			}
		default:
			return false
		}
		if r.Pin == "commit" {
			if r.Revision == nil || !dependencyHash(*r.Revision, 40) {
				return false
			}
		} else if r.Pin == "digest" {
			if r.Revision == nil || !strings.HasPrefix(*r.Revision, "sha256:") || !dependencyHash(strings.TrimPrefix(*r.Revision, "sha256:"), 64) {
				return false
			}
		} else if r.Revision != nil {
			return false
		}
		shown[dependencyClass(r)]++
	}
	for kind, n := range counts {
		if n < shown[kind] || n-shown[kind] > c.Declarations-len(e.References) {
			return false
		}
	}
	return true
}
