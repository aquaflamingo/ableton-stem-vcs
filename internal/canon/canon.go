// Package canon explodes a Live Set into a SID-addressed canonical directory
// tree and implodes it back into a byte-exact .als document.
//
// The canonical form stores each top-level entity (master track, and every child
// of <Tracks>) as its own file, and keeps the rest of the document as a template
// in which those entities are replaced by <!--stem:SID--> placeholders. Because
// the placeholders sit exactly where the original bytes were, imploding an
// unchanged tree reproduces the original document byte-for-byte.
package canon

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aquaflamingo/stem/internal/als"
)

// FormatVersion identifies the on-disk canonical layout.
const FormatVersion = 1

// Entity kinds.
const (
	KindMaster = "master"
	KindTrack  = "track"
	KindReturn = "return"
)

// Entity is one detachable top-level element.
type Entity struct {
	SID         string `json:"sid"`
	Kind        string `json:"kind"`
	TrackType   string `json:"track_type,omitempty"` // MidiTrack, AudioTrack, GroupTrack, ReturnTrack
	Name        string `json:"name"`
	LiveID      string `json:"live_id,omitempty"`
	File        string `json:"file"`
	Placeholder string `json:"placeholder"`
}

// Set is the set.json manifest.
type Set struct {
	Format    int               `json:"format"`
	Creator   string            `json:"creator,omitempty"`
	Version   map[string]string `json:"version"`
	Globals   map[string]string `json:"globals"`
	Entities  []Entity          `json:"entities"`
	template  []byte
	templateN string
}

// Name of the template file inside a canonical directory.
const templateFile = "document.xml"
const manifestFile = "set.json"

// SID derives a deterministic stable id from an entity's kind and Live id. When
// no Live id exists (e.g. the master track) a fallback string is used. The value
// is stable across saves because Live keeps its numeric Ids stable.
func SID(kind, liveID, fallback string) string {
	prefix := map[string]string{KindMaster: "mst", KindReturn: "ret", KindTrack: "trk"}[kind]
	if prefix == "" {
		prefix = "ent"
	}
	key := kind + "#id#" + liveID
	if liveID == "" {
		key = kind + "#" + fallback
	}
	sum := sha1.Sum([]byte(key))
	return prefix + "-" + hex.EncodeToString(sum[:6])
}

func trackType(e *als.Element) string { return e.Name }

func trackName(e *als.Element) string {
	if n := e.Child("Name"); n != nil {
		if v, ok := n.Child("EffectiveName").Attr("Value"); ok {
			return v
		}
		if v, ok := n.Child("UserName").Attr("Value"); ok {
			return v
		}
	}
	return ""
}

// Analysis is the in-memory decomposition shared by Explode and the diff engine.
type Analysis struct {
	Set      *Set
	Elements map[string]*als.Element // SID -> element
	Order    []string                // SIDs in document order
}

// Analyze decomposes a document into detachable top-level entities without
// touching disk. It is the single source of truth for entity identity.
func Analyze(doc *als.Doc) (*Analysis, error) {
	ls := doc.Root.Child("LiveSet")
	if ls == nil {
		return nil, fmt.Errorf("canon: no <LiveSet> element")
	}
	tracks := ls.Child("Tracks")
	if tracks == nil {
		return nil, fmt.Errorf("canon: no <Tracks> element")
	}

	set := &Set{
		Format:  FormatVersion,
		Version: map[string]string{},
		Globals: map[string]string{},
	}
	for _, a := range doc.Root.Attrs {
		set.Version[a.Name] = a.Value
	}
	if n := ls.Child("NextPointeeId"); n != nil {
		if v, ok := n.Attr("Value"); ok {
			set.Globals["NextPointeeId"] = v
		}
	}

	a := &Analysis{Set: set, Elements: map[string]*als.Element{}}
	add := func(e *als.Element, kind string) {
		tt := trackType(e)
		if kind == "" {
			switch tt {
			case "ReturnTrack":
				kind = KindReturn
			default:
				kind = KindTrack
			}
		}
		lid, _ := e.Attr("Id")
		name := trackName(e)
		fallback := name
		if kind == KindMaster {
			fallback = "master" // master has no Id; there is exactly one
		}
		if fallback == "" {
			fallback = fmt.Sprintf("%d", e.Start)
		}
		sid := SID(kind, lid, fallback)
		var rel string
		switch kind {
		case KindMaster:
			rel = "master.xml"
		case KindReturn:
			rel = filepath.Join("returns", sid+".xml")
		default:
			rel = filepath.Join("tracks", sid+".xml")
		}
		set.Entities = append(set.Entities, Entity{
			SID:         sid,
			Kind:        kind,
			TrackType:   tt,
			Name:        name,
			LiveID:      lid,
			File:        rel,
			Placeholder: "<!--stem:" + sid + "-->",
		})
		a.Elements[sid] = e
		a.Order = append(a.Order, sid)
	}

	if mt := ls.Child("MasterTrack"); mt != nil {
		add(mt, KindMaster)
	}
	for _, t := range tracks.Children {
		add(t, "")
	}
	return a, nil
}

// Explode writes the canonical directory tree for doc into dir.
func Explode(doc *als.Doc, dir string) (*Set, error) {
	a, err := Analyze(doc)
	if err != nil {
		return nil, err
	}
	set := a.Set

	// Element list index-aligned with set.Entities.
	elems := make([]*als.Element, len(set.Entities))
	for i, e := range set.Entities {
		elems[i] = a.Elements[e.SID]
	}

	// Build the template by replacing each element's byte span with its
	// placeholder, right to left so earlier offsets stay valid. Entities and
	// elems are index-aligned.
	template := append([]byte(nil), doc.Src...)
	orderIdx := make([]int, len(elems))
	for i := range orderIdx {
		orderIdx[i] = i
	}
	sort.Slice(orderIdx, func(x, y int) bool { return elems[orderIdx[x]].Start > elems[orderIdx[y]].Start })
	for _, i := range orderIdx {
		e := elems[i]
		ph := []byte(set.Entities[i].Placeholder)
		template = append(template[:e.Start], append(ph, template[e.End:]...)...)
	}

	// Write files.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, templateFile), template, 0o644); err != nil {
		return nil, err
	}
	for i, e := range elems {
		path := filepath.Join(dir, set.Entities[i].File)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, e.Raw(), 0o644); err != nil {
			return nil, err
		}
	}
	meta, err := json.MarshalIndent(set, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, manifestFile), append(meta, '\n'), 0o644); err != nil {
		return nil, err
	}
	set.template = template
	set.templateN = dir
	return set, nil
}

// LoadSet reads a canonical directory's set.json.
func LoadSet(dir string) (*Set, error) {
	b, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if err != nil {
		return nil, err
	}
	var s Set
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	tpl, err := os.ReadFile(filepath.Join(dir, templateFile))
	if err != nil {
		return nil, err
	}
	s.template = tpl
	s.templateN = dir
	return &s, nil
}

// Implode reconstructs the decompressed XML from a canonical directory.
func (s *Set) Implode(dir string) ([]byte, error) {
	out := append([]byte(nil), s.template...)
	for _, e := range s.Entities {
		raw, err := os.ReadFile(filepath.Join(dir, e.File))
		if err != nil {
			return nil, fmt.Errorf("canon: read %s: %w", e.File, err)
		}
		if !strings.Contains(string(out), e.Placeholder) {
			return nil, fmt.Errorf("canon: placeholder %s not found in template", e.Placeholder)
		}
		out = []byte(strings.Replace(string(out), e.Placeholder, string(raw), 1))
	}
	return out, nil
}

// ImplodeToFile reconstructs the XML and writes it as a gzipped .als.
func (s *Set) ImplodeToFile(dir, out string) error {
	xml, err := s.Implode(dir)
	if err != nil {
		return err
	}
	gz, err := als.EncodeBytes(xml)
	if err != nil {
		return err
	}
	return os.WriteFile(out, gz, 0o644)
}
