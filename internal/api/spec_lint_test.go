package api

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// cutDescriptionBaseline is how many one-line descriptions in api/openapi.yaml
// are known to have been cut short, counted as the entries each one leaves
// behind (a description cut at two commas leaves two).
//
// YAML ends an unquoted scalar at its first comma when it stands inside a flow
// mapping, so
//
//	cpus: { type: integer, description: The processors a node has, when the provider said. }
//
// describes the property as "The processors a node has" and adds a second key,
// "when the provider said.", with no value. That is valid YAML, so nothing
// complains: the document parses, the generated client builds, and the rest of
// the sentence is missing from the API reference and from the client's comments
// without anybody having removed it. Quote the description to keep all of it, as
// the lines of api/openapi.yaml that already carry a comma do:
//
//	cpus: { type: integer, description: "The processors a node has, when the provider said." }
//
// The entries counted here were written before this test and are corrected as
// the lines around them are rewritten, not in one sweep that would bury their
// history. The number only ever goes down: lower it when you quote some, and
// never raise it to let a new one through.
const cutDescriptionBaseline = 246

// valuelessEntry is a key that a flow mapping holds with nothing after it.
type valuelessEntry struct {
	line int    // where the key is in the document
	path string // the mapping that holds it, written as a JSON pointer like a $ref
	key  string
}

// jsonPointerEscape is how RFC 6901 writes a "/" or a "~" inside one segment of
// a pointer, which is how this file's own $refs would spell a path such as
// /jobs/stats.
var jsonPointerEscape = strings.NewReplacer("~", "~0", "/", "~1")

// flowEntriesWithoutAValue finds every entry of a flow mapping in a YAML
// document that was written as a bare key, which YAML reads as a key whose
// value is null.
//
// The rule is "written with nothing after it" and not "is null" so that an
// explicit null (`default: null`), which somebody chose, is left alone. It does
// not ask for a space in the key either: the tail of a cut description is
// usually a phrase, but "resolved." and "rounded." are tails too, and the
// document has some. Block mappings are skipped, because a comma does not end a
// scalar there and so cannot have cut anything.
func flowEntriesWithoutAValue(doc []byte) ([]valuelessEntry, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(doc, &root); err != nil {
		return nil, err
	}
	var found []valuelessEntry
	var walk func(n *yaml.Node, path string)
	walk = func(n *yaml.Node, path string) {
		switch n.Kind {
		case yaml.DocumentNode:
			for _, child := range n.Content {
				walk(child, path)
			}
		case yaml.SequenceNode:
			for i, child := range n.Content {
				walk(child, path+"/"+strconv.Itoa(i))
			}
		case yaml.MappingNode:
			flow := n.Style&yaml.FlowStyle != 0
			for i := 0; i+1 < len(n.Content); i += 2 {
				key, value := n.Content[i], n.Content[i+1]
				if flow && value.Kind == yaml.ScalarNode && value.Tag == "!!null" && value.Value == "" {
					found = append(found, valuelessEntry{line: key.Line, path: path, key: key.Value})
				}
				walk(value, path+"/"+jsonPointerEscape.Replace(key.Value))
			}
		}
	}
	walk(&root, "#")
	return found, nil
}

// TestNoNewDescriptionInTheSpecIsCutAtItsFirstComma stops the pattern described
// at cutDescriptionBaseline from growing. It counts and does not list the old
// ones, so the failure can show only the first few, in file order: they say what
// to look for, and the one that tipped the count is the description you wrote
// with a comma in it and no quotes.
func TestNoNewDescriptionInTheSpecIsCutAtItsFirstComma(t *testing.T) {
	spec, err := openapiSpec()
	if err != nil {
		t.Fatalf("openapiSpec: %v", err)
	}
	cut, err := flowEntriesWithoutAValue(spec)
	if err != nil {
		t.Fatalf("parsing the OpenAPI document: %v", err)
	}
	if len(cut) <= cutDescriptionBaseline {
		return
	}

	var examples []string
	for _, e := range cut[:min(len(cut), 5)] {
		examples = append(examples, fmt.Sprintf("api/openapi.yaml:%d, %s, has the key %q and nothing after it", e.line, e.path, e.key))
	}
	t.Errorf("api/openapi.yaml has %d flow-mapping entries with no value, up from %d. "+
		"That is what a one-line description leaves behind when it has a comma in it and no quotes: "+
		"YAML ends the text at the comma and reads the rest as a key with nothing after it, "+
		"so the sentence is cut short in the API reference and in the generated client. "+
		"Put the description in double quotes, as its neighbours do. "+
		"The first %d, to show what to look for:\n  %s",
		len(cut), cutDescriptionBaseline, len(examples), strings.Join(examples, "\n  "))
}

// The count is only as good as the rule under it, so the rule is held to the
// cases that decided it: a description cut in each of the shapes the document
// has them in, and the YAML beside it that is fine.
func TestACutDescriptionIsToldApartFromYamlThatMerelyLooksLikeOne(t *testing.T) {
	for _, tc := range []struct {
		name string
		doc  string
		want []string // each as "path: key"
	}{
		{"a comma ends the text and the rest is a key with no value",
			`a: { type: string, description: Where approving sends you, and why }`,
			[]string{"#/a: and why"}},
		{"a tail of one word has no space and is still a tail",
			`a: { type: integer, description: The count, rounded. }`,
			[]string{"#/a: rounded."}},
		{"every further comma leaves one more entry",
			`a: { description: One, two, three }`,
			[]string{"#/a: two", "#/a: three"}},
		{"a quoted description is whole",
			`a: { type: string, description: "Where approving sends you, and why" }`,
			nil},
		{"an explicit null is a value somebody wrote",
			`a: { type: [string, "null"], default: null, example: ~ }`,
			nil},
		{"a flow sequence is not a mapping",
			`a: { type: string, enum: [one, two], required: [x, y] }`,
			nil},
		{"an empty value in a block mapping cannot have been cut by a comma",
			"a:\n  b:\n  c: d\n",
			nil},
		{"a slash in a key is escaped the way a $ref spells it",
			"paths:\n  /jobs/stats:\n    get: { description: Foo, bar }\n",
			[]string{"#/paths/~1jobs~1stats/get: bar"}},
		{"a list entry is named by its position",
			"parameters:\n  - { name: a, description: Foo }\n  - { name: b, description: Foo, bar }\n",
			[]string{"#/parameters/1: bar"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			found, err := flowEntriesWithoutAValue([]byte(tc.doc))
			if err != nil {
				t.Fatalf("parsing %q: %v", tc.doc, err)
			}
			var got []string
			for _, e := range found {
				got = append(got, e.path+": "+e.key)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("%q: found %q, want %q", tc.doc, got, tc.want)
			}
		})
	}
}
