package als

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func loadFixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "small.xml"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return b
}

func TestParseBytesExact(t *testing.T) {
	src := loadFixture(t)
	d, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if d.Root.Name != "Ableton" {
		t.Fatalf("root = %q, want Ableton", d.Root.Name)
	}
	if !bytes.Equal(d.Bytes(), src) {
		t.Fatal("Bytes() is not byte-exact")
	}
}

func TestChildrenAndAttrs(t *testing.T) {
	d, err := Parse(loadFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	ls := d.Root.Child("LiveSet")
	if ls == nil {
		t.Fatal("no LiveSet")
	}
	tracks := ls.Child("Tracks")
	if got := len(tracks.Children); got != 3 {
		t.Fatalf("track count = %d, want 3", got)
	}
	at := tracks.Child("AudioTrack")
	if v, ok := at.Attr("Id"); !ok || v != "1" {
		t.Fatalf("AudioTrack Id = %q, %v", v, ok)
	}
	if got := at.Child("Name").Child("EffectiveName"); got == nil {
		t.Fatal("missing EffectiveName")
	} else if v, _ := got.Attr("Value"); v != "Drums" {
		t.Fatalf("name = %q", v)
	}
}

func TestGzipRoundTrip(t *testing.T) {
	src := loadFixture(t)
	gz, err := EncodeBytes(src)
	if err != nil {
		t.Fatal(err)
	}
	if !isGzip(gz) {
		t.Fatal("not gzip")
	}
	back, err := DecodeBytes(gz)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(back, src) {
		t.Fatal("gzip round trip mismatch")
	}
	// plain passthrough
	plain, err := DecodeBytes(src)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plain, src) {
		t.Fatal("plain passthrough mismatch")
	}
}

func TestRawSlicing(t *testing.T) {
	d, err := Parse(loadFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	at := d.Root.Child("LiveSet").Child("Tracks").Child("AudioTrack")
	raw := at.Raw()
	if !hasPrefix(raw, 0, "<AudioTrack Id=\"1\">") {
		t.Fatalf("raw starts %q", string(raw[:30]))
	}
	if string(raw[len(raw)-len("</AudioTrack>"):]) != "</AudioTrack>" {
		t.Fatal("raw does not end at close tag")
	}
}

func TestMalformed(t *testing.T) {
	cases := []string{
		"<Ableton><LiveSet></Ableton>",
		"<Ableton><LiveSet>",
		"",
		"<Ableton x=1></Ableton>",
	}
	for _, c := range cases {
		if _, err := Parse([]byte(c)); err == nil {
			t.Errorf("expected error parsing %q", c)
		}
	}
}
