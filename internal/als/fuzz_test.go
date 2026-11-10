package als

import "testing"

// FuzzParse ensures the parser never panics and that a successful parse always
// produces a byte-exact document.
func FuzzParse(f *testing.F) {
	f.Add([]byte("<Ableton><LiveSet><Tracks/></LiveSet></Ableton>"))
	f.Add([]byte(`<A Id="1" Name="x"><B/></A>`))
	f.Add([]byte("<!-- c --><a>&amp;</a>"))
	f.Add([]byte("<?xml version=\"1.0\"?><a><![CDATA[x]]></a>"))
	f.Fuzz(func(t *testing.T, data []byte) {
		d, err := Parse(data)
		if err != nil {
			return
		}
		if string(d.Bytes()) != string(data) {
			t.Fatalf("Bytes() not byte-exact for %q", data)
		}
	})
}
