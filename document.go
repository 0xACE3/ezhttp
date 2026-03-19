package fetch

import (
	"reflect"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Document wraps a parsed HTML document for CSS-selector based querying.
type Document struct {
	sel *goquery.Selection
}

func newDocument(sel *goquery.Selection) *Document {
	return &Document{sel: sel}
}

// Find returns the first element matching the CSS selector.
func (d *Document) Find(selector string) Node {
	return Node{sel: d.sel.Find(selector).First()}
}

// FindAll returns all elements matching the CSS selector.
func (d *Document) FindAll(selector string) Nodes {
	return Nodes{sel: d.sel.Find(selector)}
}

// --- Node (single element) ---

// Node represents a single HTML element.
type Node struct {
	sel *goquery.Selection
}

// Exists returns true if the selector matched an element.
func (n Node) Exists() bool { return n.sel.Length() > 0 }

// Text returns the combined text content.
func (n Node) Text() string { return strings.TrimSpace(n.sel.Text()) }

// Attr returns the value of the named attribute.
func (n Node) Attr(name string) string {
	v, _ := n.sel.Attr(name)
	return v
}

// Find searches within this node.
func (n Node) Find(selector string) Node {
	return Node{sel: n.sel.Find(selector).First()}
}

// FindAll searches within this node.
func (n Node) FindAll(selector string) Nodes {
	return Nodes{sel: n.sel.Find(selector)}
}

// Parent returns the parent element.
func (n Node) Parent() Node { return Node{sel: n.sel.Parent()} }

// Children returns direct children.
func (n Node) Children() Nodes { return Nodes{sel: n.sel.Children()} }

// Next returns the next sibling element.
func (n Node) Next() Node { return Node{sel: n.sel.Next()} }

// Prev returns the previous sibling element.
func (n Node) Prev() Node { return Node{sel: n.sel.Prev()} }

// Each iterates over children. For single Node, runs once if exists.
func (n Node) Each(fn func(Node)) {
	if n.Exists() {
		fn(n)
	}
}

// Decode populates a struct using css struct tags.
//
//	type Product struct {
//	    Name  string `css:".name"`
//	    Price string `css:".price"`
//	    SKU   string `css:".sku" attr:"data-id"`
//	}
func (n Node) Decode(dst any) error {
	return decodeNode(n.sel, dst)
}

// --- Nodes (multiple elements) ---

// Nodes represents multiple HTML elements.
type Nodes struct {
	sel *goquery.Selection
}

// Len returns the number of matched elements.
func (ns Nodes) Len() int { return ns.sel.Length() }

// Exists returns true if any elements matched.
func (ns Nodes) Exists() bool { return ns.sel.Length() > 0 }

// Text returns the text of every matched element.
func (ns Nodes) Text() []string {
	var out []string
	ns.sel.Each(func(_ int, s *goquery.Selection) {
		out = append(out, strings.TrimSpace(s.Text()))
	})
	return out
}

// Attr returns the named attribute of every matched element.
func (ns Nodes) Attr(name string) []string {
	var out []string
	ns.sel.Each(func(_ int, s *goquery.Selection) {
		if v, ok := s.Attr(name); ok {
			out = append(out, v)
		}
	})
	return out
}

// Each iterates over matched elements.
func (ns Nodes) Each(fn func(Node)) {
	ns.sel.Each(func(_ int, s *goquery.Selection) {
		fn(Node{sel: s})
	})
}

// First returns the first matched element.
func (ns Nodes) First() Node { return Node{sel: ns.sel.First()} }

// Last returns the last matched element.
func (ns Nodes) Last() Node { return Node{sel: ns.sel.Last()} }

// At returns the element at index i.
func (ns Nodes) At(i int) Node { return Node{sel: ns.sel.Eq(i)} }

// Decode populates a slice of structs using css struct tags.
// Each matched element becomes one struct in the slice.
//
//	var products []Product
//	doc.FindAll(".product-card").Decode(&products)
func (ns Nodes) Decode(dst any) error {
	dstVal := reflect.ValueOf(dst)
	if dstVal.Kind() != reflect.Ptr || dstVal.Elem().Kind() != reflect.Slice {
		return &DecodeError{Msg: "dst must be a pointer to a slice of structs"}
	}
	sliceVal := dstVal.Elem()
	elemType := sliceVal.Type().Elem()

	ns.sel.Each(func(_ int, s *goquery.Selection) {
		elem := reflect.New(elemType)
		if err := decodeNode(s, elem.Interface()); err == nil {
			sliceVal = reflect.Append(sliceVal, elem.Elem())
		}
	})
	dstVal.Elem().Set(sliceVal)
	return nil
}

// --- CSS struct tag decoder ---

// DecodeError is returned when struct decoding fails.
type DecodeError struct {
	Msg string
}

func (e *DecodeError) Error() string { return "fetch: decode: " + e.Msg }

func decodeNode(sel *goquery.Selection, dst any) error {
	v := reflect.ValueOf(dst)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return &DecodeError{Msg: "dst must be a struct or pointer to struct"}
	}
	t := v.Type()
	for i := range t.NumField() {
		field := t.Field(i)
		cssTag := field.Tag.Get("css")
		if cssTag == "" {
			continue
		}
		attrTag := field.Tag.Get("attr")

		found := sel.Find(cssTag).First()
		if found.Length() == 0 {
			continue
		}

		var val string
		if attrTag != "" {
			val, _ = found.Attr(attrTag)
		} else {
			val = strings.TrimSpace(found.Text())
		}
		if v.Field(i).CanSet() && v.Field(i).Kind() == reflect.String {
			v.Field(i).SetString(val)
		}
	}
	return nil
}
