package canon

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/aquaflamingo/stem/internal/als"
)

// corpusPath is a real Live Set kept out of git; the test skips if it is absent.
var corpusPath = filepath.Join("..", "..", "corpus", "supreme-something", "supreme something.als")

func TestRealSetRoundTrip(t *testing.T) {
	if _, err := os.Stat(corpusPath); err != nil {
		t.Skip("corpus Set not present")
	}
	d, err := als.Load(corpusPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	dir := t.TempDir()
	set, err := Explode(d, dir)
	if err != nil {
		t.Fatalf("explode: %v", err)
	}
	if len(set.Entities) < 20 {
		t.Fatalf("expected 20+ entities, got %d", len(set.Entities))
	}
	out, err := set.Implode(dir)
	if err != nil {
		t.Fatalf("implode: %v", err)
	}
	if !bytes.Equal(out, d.Src) {
		t.Fatalf("real Set implode not byte-exact (%d vs %d bytes)", len(out), len(d.Src))
	}
}
