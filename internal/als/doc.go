package als

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
)

// magicGzip is the gzip header "\x1f\x8b".
func isGzip(b []byte) bool { return len(b) >= 2 && b[0] == 0x1f && b[1] == 0x8b }

// DecodeBytes decompresses a raw .als payload if it is gzipped, otherwise
// returns it unchanged (some tools store plain XML).
func DecodeBytes(raw []byte) ([]byte, error) {
	if !isGzip(raw) {
		return raw, nil
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("als: gzip: %w", err)
	}
	defer zr.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, zr); err != nil {
		return nil, fmt.Errorf("als: gunzip: %w", err)
	}
	return buf.Bytes(), nil
}

// EncodeBytes gzip-compresses XML. Compression is deterministic for a fixed
// input and settings, which keeps the encoder reproducible.
func EncodeBytes(xml []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := zw.Write(xml); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Load reads and parses a .als from disk.
func Load(path string) (*Doc, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	xml, err := DecodeBytes(raw)
	if err != nil {
		return nil, err
	}
	return Parse(xml)
}

// Bytes returns the decompressed XML. If the document is unmodified this is the
// exact input bytes, giving a byte-exact round trip.
func (d *Doc) Bytes() []byte { return d.Src }

// Save writes the document as a gzipped .als.
func (d *Doc) Save(path string) error {
	gz, err := EncodeBytes(d.Src)
	if err != nil {
		return err
	}
	return os.WriteFile(path, gz, 0o644)
}

// Raw returns the exact source bytes of an element, including its tags.
func (e *Element) Raw() []byte {
	if e.doc == nil {
		return nil
	}
	return e.doc.Src[e.Start:e.End]
}

// Text returns the raw character data between an element's tags. It is non-empty
// only for leaf elements.
func (e *Element) Text() string {
	if e.doc == nil || e.SelfClosing || len(e.Children) > 0 {
		return ""
	}
	return string(e.doc.Src[e.HeadEnd:e.TailStart])
}

// Attr returns the value of a named attribute and whether it was present.
func (e *Element) Attr(name string) (string, bool) {
	for _, a := range e.Attrs {
		if a.Name == name {
			return a.Value, true
		}
	}
	return "", false
}

// Child returns the first direct child with the given name.
func (e *Element) Child(name string) *Element {
	for _, c := range e.Children {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// ChildrenNamed returns all direct children with the given name.
func (e *Element) ChildrenNamed(name string) []*Element {
	var out []*Element
	for _, c := range e.Children {
		if c.Name == name {
			out = append(out, c)
		}
	}
	return out
}

// Walk visits e and all descendants in document order. If fn returns false the
// children of the current element are skipped.
func (e *Element) Walk(fn func(*Element) bool) {
	if e == nil {
		return
	}
	if !fn(e) {
		return
	}
	for _, c := range e.Children {
		c.Walk(fn)
	}
}

// Depth returns the number of ancestors above e.
func (e *Element) Depth() int {
	n := 0
	for p := e.Parent; p != nil; p = p.Parent {
		n++
	}
	return n
}
