// Package diff produces a semantic, entity-level comparison of two Live Sets.
//
// It keys changes by SID (via canon.Analyze) rather than by XML position, so
// renames, reorders and edits are reported as such instead of as blob changes.
package diff

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/aquaflamingo/stem/internal/als"
	"github.com/aquaflamingo/stem/internal/canon"
)

// Change is one semantic difference. Kind groups changes for tooling; Text is a
// human-readable rendering.
type Change struct {
	Kind string `json:"kind"`
	SID  string `json:"sid,omitempty"`
	Text string `json:"text"`
}

func hash(b []byte) string {
	s := sha1.Sum(b)
	return hex.EncodeToString(s[:])
}

func findFirst(e *als.Element, name string) *als.Element {
	if e == nil {
		return nil
	}
	if e.Name == name {
		return e
	}
	for _, c := range e.Children {
		if r := findFirst(c, name); r != nil {
			return r
		}
	}
	return nil
}

func collect(e *als.Element, names map[string]bool, out *[]*als.Element) {
	if names[e.Name] {
		*out = append(*out, e)
	}
	for _, c := range e.Children {
		collect(c, names, out)
	}
}

func namedValue(e *als.Element) string {
	if e == nil {
		return ""
	}
	if v, ok := e.Attr("Value"); ok {
		return v
	}
	if v, ok := e.Attr("EffectiveName"); ok {
		return v
	}
	return ""
}

func deviceKey(e *als.Element) string { return e.Name }

func deviceName(e *als.Element) string {
	if u := e.Child("UserName"); u != nil {
		if v, ok := u.Attr("Value"); ok && v != "" {
			return e.Name + " (" + v + ")"
		}
	}
	return e.Name
}

func clipName(e *als.Element) string {
	if v := namedValue(e.Child("Name")); v != "" {
		return v
	}
	if v := namedValue(e.Child("EffectiveName")); v != "" {
		return v
	}
	return e.Name
}

func topDevices(track *als.Element) []*als.Element {
	d := findFirst(track, "Devices")
	if d == nil {
		return nil
	}
	return d.Children
}

func allClips(track *als.Element) []*als.Element {
	var out []*als.Element
	collect(track, map[string]bool{"MidiClip": true, "AudioClip": true}, &out)
	return out
}

// Compare returns the change list turning base a into changed b.
func Compare(a, b *als.Doc) ([]Change, error) {
	aa, err := canon.Analyze(a)
	if err != nil {
		return nil, err
	}
	ab, err := canon.Analyze(b)
	if err != nil {
		return nil, err
	}

	var out []Change

	// Version attributes.
	keys := map[string]bool{}
	for k := range aa.Set.Version {
		keys[k] = true
	}
	for k := range ab.Set.Version {
		keys[k] = true
	}
	for _, k := range sortedKeys(keys) {
		av, bv := aa.Set.Version[k], ab.Set.Version[k]
		if av != bv {
			out = append(out, Change{Kind: "version", Text: fmt.Sprintf("~ %s: %s -> %s", k, orDash(av), orDash(bv))})
		}
	}

	// Tempo.
	if at, bt := tempo(a), tempo(b); at != "" && bt != "" && at != bt {
		out = append(out, Change{Kind: "tempo", Text: fmt.Sprintf("~ tempo %s -> %s BPM", at, bt)})
	}

	byA := map[string]canon.Entity{}
	byB := map[string]canon.Entity{}
	for _, e := range aa.Set.Entities {
		byA[e.SID] = e
	}
	for _, e := range ab.Set.Entities {
		byB[e.SID] = e
	}

	// Added / removed.
	for _, sid := range ab.Order {
		if _, ok := byA[sid]; !ok {
			e := byB[sid]
			out = append(out, Change{Kind: "track.added", SID: sid,
				Text: fmt.Sprintf("+ %s '%s' (%s)", label(e.Kind), e.Name, e.TrackType)})
		}
	}
	for _, sid := range aa.Order {
		if _, ok := byB[sid]; !ok {
			e := byA[sid]
			out = append(out, Change{Kind: "track.removed", SID: sid,
				Text: fmt.Sprintf("- %s '%s' (%s)", label(e.Kind), e.Name, e.TrackType)})
		}
	}

	// Renamed / reordered / modified for entities present in both.
	posA := indexOf(aa.Order)
	posB := indexOf(ab.Order)
	for _, sid := range aa.Order {
		eb, ok := byB[sid]
		if !ok {
			continue
		}
		ea := byA[sid]
		if ea.Name != eb.Name {
			out = append(out, Change{Kind: "track.renamed", SID: sid,
				Text: fmt.Sprintf("~ %s renamed '%s' -> '%s'", label(ea.Kind), ea.Name, eb.Name)})
		}
		if posA[sid] != posB[sid] {
			out = append(out, Change{Kind: "track.moved", SID: sid,
				Text: fmt.Sprintf("~ %s '%s' moved %d -> %d", label(ea.Kind), eb.Name, posA[sid], posB[sid])})
		}
		if hash(aa.Elements[sid].Raw()) != hash(ab.Elements[sid].Raw()) {
			out = append(out, Change{Kind: "track.modified", SID: sid,
				Text: fmt.Sprintf("~ %s '%s' modified", label(ea.Kind), eb.Name)})
			out = append(out, compareContents(sid, aa.Elements[sid], ab.Elements[sid])...)
		}
	}
	return out, nil
}

func compareContents(sid string, a, b *als.Element) []Change {
	var out []Change
	out = append(out, diffChildren(sid, "device", topDevices(a), topDevices(b), deviceKey, deviceName)...)
	out = append(out, diffChildren(sid, "clip", allClips(a), allClips(b), clipName, clipName)...)
	return out
}

// diffChildren matches two element lists by an identity key, in order. Elements
// unmatched on one side are reported as added/removed; matched elements whose
// subtree differs are reported as changed.
func diffChildren(sid, noun string, a, b []*als.Element, keyOf, nameOf func(*als.Element) string) []Change {
	var out []Change
	usedA := make([]bool, len(a))
	for _, eb := range b {
		kb := keyOf(eb)
		matched := false
		for i, ea := range a {
			if usedA[i] || keyOf(ea) != kb {
				continue
			}
			usedA[i] = true
			matched = true
			if hash(ea.Raw()) != hash(eb.Raw()) {
				out = append(out, Change{Kind: noun + ".modified", SID: sid,
					Text: fmt.Sprintf("    ~ %s '%s' changed", noun, nameOf(eb))})
			}
			break
		}
		if !matched {
			out = append(out, Change{Kind: noun + ".added", SID: sid,
				Text: fmt.Sprintf("    + %s '%s'", noun, nameOf(eb))})
		}
	}
	for i, ea := range a {
		if !usedA[i] {
			out = append(out, Change{Kind: noun + ".removed", SID: sid,
				Text: fmt.Sprintf("    - %s '%s'", noun, nameOf(ea))})
		}
	}
	return out
}

func tempo(d *als.Doc) string {
	ls := d.Root.Child("LiveSet")
	if ls == nil {
		return ""
	}
	t := findFirst(ls, "Tempo")
	if t == nil {
		return ""
	}
	if m := t.Child("Manual"); m != nil {
		if v, ok := m.Attr("Value"); ok {
			return v
		}
	}
	return ""
}

func label(kind string) string {
	switch kind {
	case canon.KindReturn:
		return "return"
	case canon.KindMaster:
		return "master"
	default:
		return "track"
	}
}

func indexOf(order []string) map[string]int {
	m := make(map[string]int, len(order))
	for i, s := range order {
		m[s] = i
	}
	return m
}

func sortedKeys(m map[string]bool) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// Format renders changes as a list of lines.
func Format(changes []Change) string {
	var b strings.Builder
	for _, c := range changes {
		b.WriteString(c.Text)
		b.WriteByte('\n')
	}
	return b.String()
}
