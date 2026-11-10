package als

import (
	"os"
	"path/filepath"
	"testing"
)

var corpusPath = filepath.Join("..", "..", "corpus", "supreme-something", "supreme something.als")

func BenchmarkParseReal(b *testing.B) {
	if _, err := os.Stat(corpusPath); err != nil {
		b.Skip("corpus Set not present")
	}
	raw, err := os.ReadFile(corpusPath)
	if err != nil {
		b.Fatal(err)
	}
	xml, err := DecodeBytes(raw)
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(xml)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Parse(xml); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeReal(b *testing.B) {
	if _, err := os.Stat(corpusPath); err != nil {
		b.Skip("corpus Set not present")
	}
	raw, err := os.ReadFile(corpusPath)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := DecodeBytes(raw); err != nil {
			b.Fatal(err)
		}
	}
}
