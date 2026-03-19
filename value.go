package fetch

import (
	"encoding/json"
	"strings"

	"github.com/tidwall/gjson"
)

// Value provides zero-alloc JSON traversal without unmarshaling.
type Value struct {
	result gjson.Result
}

// Path navigates into nested JSON. Keys are joined with "." for gjson.
// Dots within keys are escaped automatically.
func (v Value) Path(keys ...string) Value {
	if !v.result.Exists() {
		return Value{}
	}
	escaped := make([]string, len(keys))
	for i, k := range keys {
		escaped[i] = strings.ReplaceAll(k, ".", `\.`)
	}
	return Value{result: v.result.Get(strings.Join(escaped, "."))}
}

func (v Value) String() string  { return v.result.String() }
func (v Value) Int() int64      { return v.result.Int() }
func (v Value) Float() float64  { return v.result.Float() }
func (v Value) Bool() bool      { return v.result.Bool() }
func (v Value) Raw() string     { return v.result.Raw }
func (v Value) Exists() bool    { return v.result.Exists() }

// Decode unmarshals this value's raw JSON into dst.
func (v Value) Decode(dst any) error {
	if !v.result.Exists() {
		return nil
	}
	return json.Unmarshal([]byte(v.result.Raw), dst)
}

// Array returns each element as a Value.
func (v Value) Array() []Value {
	arr := v.result.Array()
	out := make([]Value, len(arr))
	for i, r := range arr {
		out[i] = Value{result: r}
	}
	return out
}

// Keys returns object keys.
func (v Value) Keys() []string {
	if !v.result.IsObject() {
		return nil
	}
	var keys []string
	v.result.ForEach(func(key, _ gjson.Result) bool {
		keys = append(keys, key.String())
		return true
	})
	return keys
}

// Each iterates array elements.
func (v Value) Each(fn func(i int, v Value)) {
	for i, r := range v.result.Array() {
		fn(i, Value{result: r})
	}
}

// valueFromBytes parses raw JSON bytes into a Value.
func valueFromBytes(b []byte) Value {
	return Value{result: gjson.ParseBytes(b)}
}
