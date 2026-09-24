// Package htmltest parses HTML responses and finds elements in them, for tests.
package htmltest

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// Doc is a parsed HTML document or fragment.
type Doc struct {
	t    testing.TB
	Root *html.Node
}

// Parse parses body, failing the test when it is not HTML. A fragment is parsed as the body of a document.
func Parse(t testing.TB, body string) Doc {
	t.Helper()
	root, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse HTML: %v", err)
	}
	return Doc{t: t, Root: root}
}

// All returns every element under n for which match is true, in document order.
func All(n *html.Node, match func(*html.Node) bool) []*html.Node {
	var out []*html.Node
	for d := range n.Descendants() {
		if d.Type == html.ElementNode && match(d) {
			out = append(out, d)
		}
	}
	return out
}

// ByID returns the element with the id, failing the test when there is none or more than one.
func (d Doc) ByID(id string) *html.Node {
	d.t.Helper()
	found := All(d.Root, func(n *html.Node) bool { return Attr(n, "id") == id })
	if len(found) != 1 {
		d.t.Fatalf("found %d elements with id %q, want 1", len(found), id)
	}
	return found[0]
}

// HasID reports whether an element with the id exists.
func (d Doc) HasID(id string) bool {
	return len(All(d.Root, func(n *html.Node) bool { return Attr(n, "id") == id })) > 0
}

// Tags returns every element with the tag name.
func (d Doc) Tags(name string) []*html.Node {
	return All(d.Root, func(n *html.Node) bool { return n.Data == name })
}

// One returns the only element with the tag name.
func (d Doc) One(name string) *html.Node {
	d.t.Helper()
	found := d.Tags(name)
	if len(found) != 1 {
		d.t.Fatalf("found %d <%s> elements, want 1", len(found), name)
	}
	return found[0]
}

// Attr returns the value of the attribute, or "" when n does not have it.
func Attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

// HasAttr reports whether n has the attribute.
func HasAttr(n *html.Node, name string) bool {
	for _, a := range n.Attr {
		if a.Key == name {
			return true
		}
	}
	return false
}

// Text returns the element's text with whitespace runs collapsed.
func Text(n *html.Node) string {
	var b strings.Builder
	for d := range n.Descendants() {
		if d.Type == html.TextNode {
			b.WriteString(d.Data)
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// Within reports whether n is inside ancestor.
func Within(n, ancestor *html.Node) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p == ancestor {
			return true
		}
	}
	return false
}
