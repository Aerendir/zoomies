package workflow

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// CheckoutDepthChange preserves the original line, including comments and line
// endings. Unsupported syntax is refused rather than reserialised as new YAML.
func CheckoutDepthChange(data []byte, jobIndex, stepIndex int) (before, after string, line int, err error) {
	if _, err = Inspect(data); err != nil {
		return
	}
	root, parseErr := parseRoot(data)
	if parseErr != nil {
		err = parseErr
		return
	}
	jobs := field(root, "jobs")
	if jobIndex < 0 || jobIndex >= len(jobs.Content)/2 {
		err = errUnsupported
		return
	}
	job := jobs.Content[jobIndex*2+1]
	steps := field(job, "steps")
	if field(job, "uses") != nil || steps == nil || steps.Kind != yaml.SequenceNode || stepIndex < 0 || stepIndex >= len(steps.Content) {
		err = errUnsupported
		return
	}
	step := steps.Content[stepIndex]
	uses := field(step, "uses")
	if field(step, "run") != nil || uses == nil || uses.Kind != yaml.ScalarNode || !strings.HasPrefix(uses.Value, "actions/checkout@") || !commitSHA.MatchString(strings.TrimPrefix(uses.Value, "actions/checkout@")) {
		err = errUnsupported
		return
	}
	with := field(step, "with")
	if with == nil || with.Kind != yaml.MappingNode || with.Style&yaml.FlowStyle != 0 {
		err = errUnsupported
		return
	}
	depth := field(with, "fetch-depth")
	if depth == nil || depth.Kind != yaml.ScalarNode || depth.Value != "0" || depth.Style != 0 || depth.Tag != "!!int" || field(with, "fetch-tags") != nil || field(with, "ref") != nil || field(with, "repository") != nil {
		err = errUnsupported
		return
	}
	lines := strings.SplitAfter(string(data), "\n")
	line = depth.Line
	if line < 1 || line > len(lines) {
		err = errUnsupported
		return
	}
	before = lines[line-1]
	offset := depth.Column - 1
	if offset < 0 || offset >= len(before) || before[offset] != '0' || strings.Count(string(data), before) != 1 {
		err = errUnsupported
		return
	}
	after = before[:offset] + "1" + before[offset+1:]
	return
}
