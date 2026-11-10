package repo

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/aquaflamingo/stem/internal/als"
)

func TestInitAndRebuild(t *testing.T) {
	dir := t.TempDir()
	src, err := os.ReadFile(filepath.Join("..", "..", "testdata", "small.xml"))
	if err != nil {
		t.Fatal(err)
	}
	setPath := filepath.Join(dir, "Song.als")
	if err := os.WriteFile(setPath, src, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Init(dir); err != nil {
		t.Fatalf("init: %v", err)
	}
	for _, f := range []string{"stem.toml", ".gitignore", ".gitattributes", "set/set.json", "set/document.xml", ".git"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s after init: %v", f, err)
		}
	}

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SetFile != "Song.als" {
		t.Fatalf("set_file = %q", cfg.SetFile)
	}

	// Remove the working Set and rebuild it from the canonical tree.
	if err := os.Remove(setPath); err != nil {
		t.Fatal(err)
	}
	out, err := Rebuild(dir)
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	d, err := als.Load(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(d.Bytes(), src) {
		t.Fatal("rebuilt Set is not byte-identical to the original")
	}
}

func TestFindSetRejectsMultiple(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.als", "b.als"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("<Ableton/>"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Init(dir); err == nil {
		t.Fatal("expected error for multiple .als files")
	}
}
