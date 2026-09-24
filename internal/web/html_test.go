package web_test

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// doc is a parsed HTML response.
type doc struct {
	t    *testing.T
	root *html.Node
}

func parseHTML(t *testing.T, body string) doc {
	t.Helper()
	root, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse HTML: %v", err)
	}
	return doc{t: t, root: root}
}

// all returns every element under n for which match is true, in document order.
func all(n *html.Node, match func(*html.Node) bool) []*html.Node {
	var out []*html.Node
	for d := range n.Descendants() {
		if d.Type == html.ElementNode && match(d) {
			out = append(out, d)
		}
	}
	return out
}

// byID returns the element with the id, failing the test when there is none or more than one.
func (d doc) byID(id string) *html.Node {
	d.t.Helper()
	found := all(d.root, func(n *html.Node) bool { return attr(n, "id") == id })
	if len(found) != 1 {
		d.t.Fatalf("found %d elements with id %q, want 1", len(found), id)
	}
	return found[0]
}

// hasID reports whether an element with the id exists.
func (d doc) hasID(id string) bool {
	return len(all(d.root, func(n *html.Node) bool { return attr(n, "id") == id })) > 0
}

// tags returns every element with the tag name.
func (d doc) tags(name string) []*html.Node {
	return all(d.root, func(n *html.Node) bool { return n.Data == name })
}

// one returns the only element with the tag name.
func (d doc) one(name string) *html.Node {
	d.t.Helper()
	found := d.tags(name)
	if len(found) != 1 {
		d.t.Fatalf("found %d <%s> elements, want 1", len(found), name)
	}
	return found[0]
}

func attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

func hasAttr(n *html.Node, name string) bool {
	for _, a := range n.Attr {
		if a.Key == name {
			return true
		}
	}
	return false
}

// text returns the element's text with whitespace runs collapsed.
func text(n *html.Node) string {
	var b strings.Builder
	for d := range n.Descendants() {
		if d.Type == html.TextNode {
			b.WriteString(d.Data)
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// within reports whether n is inside ancestor.
func within(n, ancestor *html.Node) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p == ancestor {
			return true
		}
	}
	return false
}
