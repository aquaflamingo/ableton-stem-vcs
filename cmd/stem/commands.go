package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/aquaflamingo/stem/internal/als"
	"github.com/aquaflamingo/stem/internal/canon"
	"github.com/aquaflamingo/stem/internal/diff"
	"github.com/aquaflamingo/stem/internal/repo"
)

func cmdInit(args []string) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	setFile, err := repo.Init(dir)
	if err != nil {
		return err
	}
	fmt.Printf("initialised Stem repo in %s\n", dir)
	fmt.Printf("adopted Set:  %s\n", filepath.Base(setFile))
	fmt.Printf("canonical:    %s/ (exploded, ready to commit)\n", filepath.Join(dir, "set"))
	fmt.Printf("wrote:        stem.toml, .gitignore, .gitattributes\n")
	fmt.Printf("next:         git commit -m 'stem init'  (or 'stem rebuild' to restore the .als)\n")
	return nil
}

func cmdRebuild(args []string) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	out, err := repo.Rebuild(dir)
	if err != nil {
		return err
	}
	fmt.Printf("rebuilt %s\n", out)
	return nil
}

func cmdInfo(args []string) error {
	args = requireArgs(args, 1, "info <set.als>")
	d, err := loadDoc(args[0])
	if err != nil {
		return err
	}
	ls := d.Root.Child("LiveSet")
	creator, _ := d.Root.Attr("Creator")
	minor, _ := d.Root.Attr("MinorVersion")
	next, _ := func() (string, bool) {
		if ls == nil {
			return "", false
		}
		if n := ls.Child("NextPointeeId"); n != nil {
			return n.Attr("Value")
		}
		return "", false
	}()

	a, err := canon.Analyze(d)
	if err != nil {
		return err
	}
	kinds := map[string]int{}
	var tracks int
	var returns int
	var master int
	for _, e := range a.Set.Entities {
		switch e.Kind {
		case canon.KindMaster:
			master++
		case canon.KindReturn:
			returns++
		default:
			tracks++
		}
		kinds[e.TrackType]++
	}

	elements := 0
	maxDepth := 0
	d.Root.Walk(func(e *als.Element) bool {
		elements++
		if dp := e.Depth(); dp > maxDepth {
			maxDepth = dp
		}
		return true
	})

	fmt.Printf("Live Set:        %s\n", filepath.Base(args[0]))
	fmt.Printf("Creator:         %s\n", creator)
	fmt.Printf("MinorVersion:    %s\n", minor)
	fmt.Printf("NextPointeeId:   %s\n", next)
	fmt.Printf("XML size:        %d bytes (decompressed)\n", len(d.Src))
	fmt.Printf("XML elements:    %d (max depth %d)\n", elements, maxDepth)
	fmt.Printf("Tracks:          %d (master %d, returns %d)\n", tracks, master, returns)

	types := make([]string, 0, len(kinds))
	for k := range kinds {
		types = append(types, k)
	}
	sort.Strings(types)
	for _, k := range types {
		fmt.Printf("  %-14s %d\n", k, kinds[k])
	}
	return nil
}

func cmdCheck(args []string) error {
	args = requireArgs(args, 1, "check <set.als>")
	raw, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	xml, err := als.DecodeBytes(raw)
	if err != nil {
		return err
	}
	d, err := als.Parse(xml)
	if err != nil {
		return err
	}
	fmt.Printf("parsed OK: %d bytes, root <%s>\n", len(d.Src), d.Root.Name)

	// Byte-exact codec round trip: the document is unmodified, so Bytes() must
	// equal the decompressed input.
	if !bytes.Equal(d.Bytes(), xml) {
		return fmt.Errorf("codec round trip not byte-exact")
	}
	fmt.Printf("codec round trip:   byte-exact\n")

	// Canonical fixed point.
	dir, err := os.MkdirTemp("", "stem-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	set, err := canon.Explode(d, dir)
	if err != nil {
		return err
	}
	fmt.Printf("explode:            %d entities -> %s\n", len(set.Entities), dir)

	out, err := set.Implode(dir)
	if err != nil {
		return err
	}
	if !bytes.Equal(out, xml) {
		return fmt.Errorf("implode not byte-exact (got %d bytes, want %d)", len(out), len(xml))
	}
	fmt.Printf("implode:            byte-exact\n")

	// Re-explode and confirm the canonical form is a fixed point.
	d2, err := als.Parse(out)
	if err != nil {
		return err
	}
	dir2, err := os.MkdirTemp("", "stem-check2-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir2)
	set2, err := canon.Explode(d2, dir2)
	if err != nil {
		return err
	}
	if err := sameCanonical(set, set2); err != nil {
		return fmt.Errorf("canonical form not a fixed point: %w", err)
	}
	fmt.Printf("canonical fixed pt: OK\n")
	return nil
}

func sameCanonical(a, b *canon.Set) error {
	if len(a.Entities) != len(b.Entities) {
		return fmt.Errorf("entity count %d != %d", len(a.Entities), len(b.Entities))
	}
	for i := range a.Entities {
		if a.Entities[i] != b.Entities[i] {
			return fmt.Errorf("entity %d differs: %+v != %+v", i, a.Entities[i], b.Entities[i])
		}
	}
	return nil
}

func cmdDecompress(args []string) error {
	args = requireArgs(args, 2, "decompress <in.als> <out.xml>")
	d, err := loadDoc(args[0])
	if err != nil {
		return err
	}
	return os.WriteFile(args[1], d.Bytes(), 0o644)
}

func cmdExplode(args []string) error {
	args = requireArgs(args, 2, "explode <in.als> <dir>")
	d, err := loadDoc(args[0])
	if err != nil {
		return err
	}
	set, err := canon.Explode(d, args[1])
	if err != nil {
		return err
	}
	fmt.Printf("exploded %d entities -> %s\n", len(set.Entities), args[1])
	return nil
}

func cmdImplode(args []string) error {
	args = requireArgs(args, 2, "implode <dir> <out.als>")
	set, err := canon.LoadSet(args[0])
	if err != nil {
		return err
	}
	if err := set.ImplodeToFile(args[0], args[1]); err != nil {
		return err
	}
	fmt.Printf("imploded %s -> %s\n", args[0], args[1])
	return nil
}

func cmdDiff(args []string) error {
	args = requireArgs(args, 2, "diff <a.als> <b.als>")
	a, err := loadDoc(args[0])
	if err != nil {
		return err
	}
	b, err := loadDoc(args[1])
	if err != nil {
		return err
	}
	changes, err := diff.Compare(a, b)
	if err != nil {
		return err
	}
	if len(changes) == 0 {
		fmt.Println("no semantic changes")
		return nil
	}
	fmt.Print(diff.Format(changes))
	fmt.Printf("\n%d change(s)\n", len(changes))
	return nil
}
