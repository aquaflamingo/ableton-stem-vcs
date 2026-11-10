// Package als reads and writes Ableton Live Set (.als) files.
//
// The .als file is gzip-compressed XML. This package keeps the decompressed XML
// byte-exact and builds a lightweight element tree whose nodes carry source
// offsets, so any subtree can be sliced out verbatim and any unmodified document
// can be re-emitted byte-for-byte.
package als

import "fmt"

// Attr is one XML attribute, preserving source order.
type Attr struct {
	Name  string
	Value string
}

// Element is a node in the parsed XML tree. Offsets index into Doc.Src.
type Element struct {
	Name        string
	Attrs       []Attr
	Children    []*Element
	Parent      *Element
	SelfClosing bool

	Start     int // offset of the '<' that opens this element
	HeadEnd   int // offset just past the '>' of the start tag
	TailStart int // offset of the '<' that closes this element (== Start if self-closing)
	End       int // offset just past the '>' of the closing tag

	doc *Doc
}

// Doc is a parsed Live Set document.
type Doc struct {
	Src  []byte // decompressed XML, byte-exact
	Root *Element
}

const maxDepth = 512

func errBadXML(off int, msg string) error {
	return fmt.Errorf("als: malformed XML at byte %d: %s", off, msg)
}

// Parse builds a Doc from decompressed XML bytes. It preserves unknown elements
// and does not validate against any Ableton schema.
func Parse(src []byte) (*Doc, error) {
	d := &Doc{Src: src}
	p := &parser{src: src}
	i := p.skipProlog(0)
	root, err := p.parseElement(i, nil, 0)
	if err != nil {
		return nil, err
	}
	d.Root = root
	linkDoc(root, d)
	return d, nil
}

func linkDoc(e *Element, d *Doc) {
	e.doc = d
	for _, c := range e.Children {
		linkDoc(c, d)
	}
}

type parser struct {
	src []byte
}

func isNameByte(c byte) bool {
	return c == '_' || c == '-' || c == '.' || c == ':' ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }

func hasPrefix(b []byte, i int, s string) bool {
	return i+len(s) <= len(b) && string(b[i:i+len(s)]) == s
}

func indexFrom(b []byte, from int, sub string) int {
	if from >= len(b) || from > len(b)-len(sub) {
		return -1
	}
	for i := from; i+len(sub) <= len(b); i++ {
		if string(b[i:i+len(sub)]) == sub {
			return i
		}
	}
	return -1
}

func (p *parser) skipProlog(i int) int {
	for i < len(p.src) {
		if p.src[i] != '<' {
			i++
			continue
		}
		switch {
		case hasPrefix(p.src, i, "<?"):
			j := indexFrom(p.src, i+2, "?>")
			if j < 0 {
				return len(p.src)
			}
			i = j + 2
		case hasPrefix(p.src, i, "<!--"):
			j := indexFrom(p.src, i+4, "-->")
			if j < 0 {
				return len(p.src)
			}
			i = j + 3
		case hasPrefix(p.src, i, "<!"):
			j := indexFrom(p.src, i, ">")
			if j < 0 {
				return len(p.src)
			}
			i = j + 1
		default:
			return i
		}
	}
	return i
}

// parseElement parses a full element (start tag, children, end tag) at i.
func (p *parser) parseElement(i int, parent *Element, depth int) (*Element, error) {
	if depth > maxDepth {
		return nil, errBadXML(i, "nesting too deep")
	}
	if i >= len(p.src) || p.src[i] != '<' || hasPrefix(p.src, i, "</") {
		return nil, errBadXML(i, "expected element start")
	}
	e := &Element{Start: i, Parent: parent}
	j := i + 1
	ns := j
	for j < len(p.src) && isNameByte(p.src[j]) {
		j++
	}
	if j == ns {
		return nil, errBadXML(i, "empty element name")
	}
	e.Name = string(p.src[ns:j])

	for {
		for j < len(p.src) && isSpace(p.src[j]) {
			j++
		}
		if j >= len(p.src) {
			return nil, errBadXML(i, "truncated start tag")
		}
		switch p.src[j] {
		case '>':
			j++
			e.HeadEnd = j
			return p.parseContent(e, j, depth)
		case '/':
			if j+1 < len(p.src) && p.src[j+1] == '>' {
				e.SelfClosing = true
				e.HeadEnd = j + 2
				e.TailStart = i
				e.End = j + 2
				return e, nil
			}
			return nil, errBadXML(j, "unexpected '/' in start tag")
		default:
			as := j
			for j < len(p.src) && isNameByte(p.src[j]) {
				j++
			}
			if j == as {
				return nil, errBadXML(j, "expected attribute name")
			}
			name := string(p.src[as:j])
			for j < len(p.src) && isSpace(p.src[j]) {
				j++
			}
			if j >= len(p.src) || p.src[j] != '=' {
				return nil, errBadXML(j, "expected '=' after attribute "+name)
			}
			j++
			for j < len(p.src) && isSpace(p.src[j]) {
				j++
			}
			if j >= len(p.src) || (p.src[j] != '"' && p.src[j] != '\'') {
				return nil, errBadXML(j, "expected quoted attribute value")
			}
			q := p.src[j]
			j++
			vs := j
			for j < len(p.src) && p.src[j] != q {
				j++
			}
			if j >= len(p.src) {
				return nil, errBadXML(vs, "unterminated attribute value")
			}
			e.Attrs = append(e.Attrs, Attr{Name: name, Value: string(p.src[vs:j])})
			j++
		}
	}
}

// parseContent walks from j (just past the start tag '>') to the matching end
// tag, appending child elements.
func (p *parser) parseContent(e *Element, j int, depth int) (*Element, error) {
	for j < len(p.src) {
		if p.src[j] != '<' {
			j++
			continue
		}
		switch {
		case hasPrefix(p.src, j, "</"):
			k := j + 2
			es := k
			for k < len(p.src) && isNameByte(p.src[k]) {
				k++
			}
			if string(p.src[es:k]) != e.Name {
				return nil, errBadXML(j, "mismatched end tag </"+string(p.src[es:k])+"> for <"+e.Name+">")
			}
			for k < len(p.src) && p.src[k] != '>' {
				k++
			}
			if k >= len(p.src) {
				return nil, errBadXML(j, "unterminated end tag")
			}
			e.TailStart = j
			e.End = k + 1
			return e, nil
		case hasPrefix(p.src, j, "<!--"):
			k := indexFrom(p.src, j+4, "-->")
			if k < 0 {
				return nil, errBadXML(j, "unterminated comment")
			}
			j = k + 3
		case hasPrefix(p.src, j, "<![CDATA["):
			k := indexFrom(p.src, j+9, "]]>")
			if k < 0 {
				return nil, errBadXML(j, "unterminated CDATA")
			}
			j = k + 3
		case hasPrefix(p.src, j, "<?"):
			k := indexFrom(p.src, j+2, "?>")
			if k < 0 {
				return nil, errBadXML(j, "unterminated processing instruction")
			}
			j = k + 2
		default:
			child, err := p.parseElement(j, e, depth+1)
			if err != nil {
				return nil, err
			}
			e.Children = append(e.Children, child)
			j = child.End
		}
	}
	return nil, errBadXML(e.Start, "unterminated element <"+e.Name+">")
}
