package proxmox

// Ownership marks: what this fleet writes on a guest so that a sweep can tell
// its own machines from everybody else's.
//
// Two places, because they answer two different questions. Tags appear in the
// one cluster-wide listing, so they are how a sweep filters thousands of guests
// down to a handful without asking each one for its configuration. The
// description holds the whole record -- controller, provider, machine,
// fingerprint, when -- and is read back from the guest's own configuration.
//
// Neither is authenticated. Any Proxmox user with configuration rights can
// write both, so nothing here is proof of anything: the store is the authority
// and these are tamper-evidence. What they catch is a recycled identifier,
// another controller's guest, and one somebody made by hand -- the three cases
// that actually happen -- and a delete requires the row and the marks to agree.

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/eyupio/zoomies/internal/provider"
)

// FleetTag is on every guest this fleet created. It is what a sweep filters on
// and what an operator sorts by in the Proxmox console.
const FleetTag = "zoomies"

// machineTagPrefix begins the per-machine tag, so a guest in the console names
// the row on the machines page without a lookup.
const machineTagPrefix = "zoomies-"

// tagSeparator is how Proxmox joins a guest's tags on the wire. It accepts
// several separators on the way in and writes this one on the way out.
const tagSeparator = ";"

// ownershipBlock is the JSON written into a guest's description. It is nested
// under one key so that a description this fleet did not write, but which
// happens to be JSON, cannot be mistaken for one of ours.
type ownershipBlock struct {
	Zoomies ownershipRecord `json:"zoomies"`
}

type ownershipRecord struct {
	Controller  string `json:"controller"`
	Provider    string `json:"provider"`
	Machine     string `json:"machine"`
	Fingerprint string `json:"fingerprint"`
	CreatedAt   string `json:"created_at,omitempty"`
}

// descriptionHeading and descriptionPreamble are what an operator reads in the
// Proxmox console before they reach anything machine-readable. A guest that
// appears in somebody else's cluster with no explanation is a guest somebody
// deletes by hand. The console renders a guest's notes as Markdown, so the
// heading is one and the preamble is a paragraph rather than one long line.
const (
	descriptionHeading  = "## 🐾 Managed by Zoomies 🐾"
	descriptionPreamble = "This guest is a rented runner machine in a Zoomies fleet. **Deleting or renaming it by hand will leave the fleet with a machine it cannot account for.**"
)

// Describe renders the description to write on a guest.
//
// The human half comes first and is Markdown: a heading, the sentence above and
// a short list of which machine this is. The machine-readable blocks follow
// inside an HTML comment, which the console's renderer drops, so an operator
// reads prose and not JSON, while each block stays one line of its own so that
// DecodeOwner finds it however an operator edits around it. Further blocks, such
// as the shape a clone was asked for, ride in the same comment.
func Describe(owner provider.Owner, name string) string {
	return describe(owner, name)
}

func describe(owner provider.Owner, name string, blocks ...string) string {
	rec := ownershipRecord{
		Controller:  owner.ControllerID,
		Provider:    owner.ProviderID,
		Machine:     owner.MachineID,
		Fingerprint: owner.Fingerprint,
	}
	facts := []string{}
	if name != "" {
		facts = append(facts, "- **Machine:** `"+name+"`")
	}
	if owner.ProviderID != "" {
		facts = append(facts, "- **Provider:** `"+owner.ProviderID+"`")
	}
	if owner.ControllerID != "" {
		facts = append(facts, "- **Controller:** `"+owner.ControllerID+"`")
	}
	if !owner.CreatedAt.IsZero() {
		// UTC, always: a hypervisor's timezone is not ours, and a mark that
		// read differently depending on where it was written would be useless
		// as evidence.
		rec.CreatedAt = owner.CreatedAt.UTC().Format(time.RFC3339Nano)
		facts = append(facts, "- **Created:** "+owner.CreatedAt.UTC().Format("2006-01-02 15:04")+" UTC")
	}
	blob, err := json.Marshal(ownershipBlock{Zoomies: rec})
	if err != nil {
		// Nothing in the record can fail to encode; a description without the
		// block is still better than no description at all.
		return descriptionHeading + "\n\n" + descriptionPreamble
	}
	parts := []string{descriptionHeading, descriptionPreamble}
	if len(facts) > 0 {
		parts = append(parts, strings.Join(facts, "\n"))
	}
	comment := append([]string{"<!--", string(blob)}, blocks...)
	parts = append(parts, strings.Join(append(comment, "-->"), "\n"))
	return strings.Join(parts, "\n\n")
}

// DecodeOwner reads the marks back out of a description, reporting whether it
// found any.
//
// It scans line by line rather than parsing the whole description, because an
// operator who appends a note to a guest's description has not thereby
// disowned it -- and a sweep that read that as "unmarked" would quarantine a
// machine the fleet is quite sure about.
func DecodeOwner(description string) (provider.Owner, bool) {
	for _, line := range strings.Split(description, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var block ownershipBlock
		if err := json.Unmarshal([]byte(line), &block); err != nil {
			continue
		}
		rec := block.Zoomies
		if rec.Controller == "" && rec.Machine == "" && rec.Fingerprint == "" {
			continue
		}
		owner := provider.Owner{
			ControllerID: rec.Controller,
			ProviderID:   rec.Provider,
			MachineID:    rec.Machine,
			Fingerprint:  rec.Fingerprint,
		}
		// A timestamp that will not parse loses the timestamp and nothing else.
		// What a delete checks is the controller, the machine and the
		// fingerprint, and refusing the whole block over a date would turn a
		// machine we own into one nobody may touch.
		if rec.CreatedAt != "" {
			if at, err := time.Parse(time.RFC3339Nano, rec.CreatedAt); err == nil {
				owner.CreatedAt = at.UTC()
			}
		}
		return owner, true
	}
	return provider.Owner{}, false
}

// Tags are the tags to write on a guest: the fleet's, and the machine's own.
func Tags(owner provider.Owner) []string {
	tags := []string{FleetTag}
	if t := MachineTag(owner.MachineID); t != "" {
		tags = append(tags, t)
	}
	return tags
}

// MachineTag is the per-machine tag, or "" for an identifier with nothing
// usable in it. The machine's own identifier follows the prefix, with its own
// prefix dropped: "mach_k3f9qz2mx7ab" is already self-describing next to
// "zoomies-", and a tag has a grammar to stay inside.
func MachineTag(machineID string) string {
	fragment := machineID
	if _, rest, ok := strings.Cut(machineID, "_"); ok && rest != "" {
		fragment = rest
	}
	if fragment = sanitiseTag(fragment); fragment == "" {
		return ""
	}
	return machineTagPrefix + fragment
}

// EncodeTags renders tags for the wire, merging in any the guest already
// carries so that a tag an operator added by hand survives being configured.
// The result is sorted, because a set written in a different order every pass
// would rewrite a guest's configuration for nothing.
func EncodeTags(existing string, add []string) string {
	seen := map[string]bool{}
	var all []string
	for _, t := range append(ParseTags(existing), add...) {
		t = sanitiseTag(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		all = append(all, t)
	}
	sort.Strings(all)
	return strings.Join(all, tagSeparator)
}

// ParseTags splits a guest's tags. Proxmox writes them separated by semicolons
// and accepts commas and spaces, so all three are read.
func ParseTags(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ';' || r == ',' || r == ' ' || r == '\t'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if t := sanitiseTag(f); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// HasFleetTag reports whether a guest's tags claim it for this fleet. It is a
// hint and not proof: it is what narrows a cluster-wide listing down to the
// guests worth reading a description for.
func HasFleetTag(tags string) bool {
	for _, t := range ParseTags(tags) {
		if t == FleetTag {
			return true
		}
	}
	return false
}

// sanitiseTag makes a tag Proxmox will accept: lower case, and nothing outside
// letters, digits, hyphen, underscore and full stop. A tag Proxmox rejects
// fails the whole configuration call, which would leave a guest running and
// unmarked -- the one state this file exists to prevent.
func sanitiseTag(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			// Dropped rather than replaced: a substitute character would make
			// two different identifiers collide on one tag.
		}
	}
	// Proxmox requires the first character to be a letter, a digit or an
	// underscore, so a tag that would start with punctuation is trimmed to
	// where it does.
	return strings.TrimLeft(b.String(), "-.")
}
