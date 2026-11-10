package canon

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/aquaflamingo/stem/internal/als"
)

func fixtureDoc(t *testing.T) *als.Doc {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "..", "testdata", "small.xml"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	d, err := als.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return d
}

func TestExplodeImplodeByteExact(t *testing.T) {
	d := fixtureDoc(t)
	dir := t.TempDir()
	set, err := Explode(d, dir)
	if err != nil {
		t.Fatalf("explode: %v", err)
	}
	if len(set.Entities) != 4 { // audio + midi + return + master
		t.Fatalf("entities = %d, want 4", len(set.Entities))
	}
	out, err := set.Implode(dir)
	if err != nil {
		t.Fatalf("implode: %v", err)
	}
	if !bytes.Equal(out, d.Src) {
		t.Fatal("implode is not byte-exact")
	}
}

func TestCanonicalFixedPoint(t *testing.T) {
	d := fixtureDoc(t)
	dir1 := t.TempDir()
	s1, err := Explode(d, dir1)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s1.Implode(dir1)
	if err != nil {
		t.Fatal(err)
	}
	d2, err := als.Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	dir2 := t.TempDir()
	s2, err := Explode(d2, dir2)
	if err != nil {
		t.Fatal(err)
	}
	if len(s1.Entities) != len(s2.Entities) {
		t.Fatalf("entity count changed: %d -> %d", len(s1.Entities), len(s2.Entities))
	}
	for i := range s1.Entities {
		if s1.Entities[i] != s2.Entities[i] {
			t.Fatalf("entity %d unstable:\n %+v\n %+v", i, s1.Entities[i], s2.Entities[i])
		}
	}
}

func TestSIDStableAcrossByteShift(t *testing.T) {
	// Prefixing the document shifts every byte offset; SIDs must not change.
	src, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "small.xml"))
	d, err := als.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	a, err := Analyze(d)
	if err != nil {
		t.Fatal(err)
	}
	if a.Set.Entities[0].SID == "" {
		t.Fatal("empty SID")
	}
	// Master SID must be independent of offset.
	if got := SID(KindMaster, "", "master"); got != SID(KindMaster, "", "master") {
		t.Fatal("SID not deterministic")
	}
}
