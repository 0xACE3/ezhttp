package ezhttp

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/tidwall/gjson"
)

// Response holds a completed HTTP response. All fields are populated even on
// HTTP errors (4xx/5xx). Transport errors leave Status=0 and Body=nil.
type Response struct {
	Status         int
	Body           []byte
	Headers        Headers
	RequestHeaders Headers
	RequestURL     string

	err error
}

// Err returns the response error: transport failure or non-2xx status.
func (r *Response) Err() error { return r.err }

// Text returns the body as a string. Returns "" on error.
func (r *Response) Text() string {
	if r.err != nil || r.Body == nil {
		return ""
	}
	return string(r.Body)
}

// Bytes returns the raw body. Returns nil on error.
func (r *Response) Bytes() []byte {
	if r.err != nil {
		return nil
	}
	return r.Body
}

// JSON unmarshals the body into dst. Returns the first error encountered:
// transport, HTTP status, or JSON parse.
func (r *Response) JSON(dst any) error {
	if r.err != nil {
		return r.err
	}
	return json.Unmarshal(r.Body, dst)
}

// HTML parses the body as HTML and returns a Document for CSS querying.
func (r *Response) HTML() (*Document, error) {
	if r.err != nil {
		return nil, r.err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(r.Body)))
	if err != nil {
		return nil, err
	}
	return newDocument(doc.Selection), nil
}

// Save writes the body to a file.
func (r *Response) Save(path string) error {
	if r.err != nil {
		return r.err
	}
	return os.WriteFile(path, r.Body, 0o644)
}

// Through applies a transform function to the body, returning the same Response
// for chaining. Skipped if there's already an error.
func (r *Response) Through(fn ThroughFunc) *Response {
	if r.err != nil {
		return r
	}
	b, err := fn(r.Body)
	if err != nil {
		r.err = err
		return r
	}
	r.Body = b
	return r
}

// Path navigates into the JSON body without unmarshaling.
//
//	price := resp.Path("data", "quote", "USD", "price").Float()
func (r *Response) Path(keys ...string) Value {
	if r.err != nil || r.Body == nil {
		return Value{}
	}
	escaped := make([]string, len(keys))
	for i, k := range keys {
		escaped[i] = strings.ReplaceAll(k, ".", `\.`)
	}
	path := strings.Join(escaped, ".")
	return Value{result: gjson.GetBytes(r.Body, path)}
}

// To unmarshals the JSON response into T using generics.
//
//	user, err := fetch.To[User](client.Get(ctx, "/users/1"))
func To[T any](r *Response) (T, error) {
	var v T
	if r.err != nil {
		return v, r.err
	}
	if err := json.Unmarshal(r.Body, &v); err != nil {
		return v, err
	}
	return v, nil
}
