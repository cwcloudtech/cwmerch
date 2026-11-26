// Package php reads and writes PHP's serialize() format, which WordPress and
// WooCommerce use for options and meta values (roles, gateway settings,
// product attributes...).
//
// Decoded values are: nil, bool, int64, float64, string, and *Array (an
// ordered map, since PHP arrays keep insertion order and mix int/string keys).
package php

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Array is an ordered PHP array. Keys are int64 or string.
type Array struct {
	Keys   []any
	Values map[string]any
}

// NewArray is an empty array.
func NewArray() *Array { return &Array{Values: map[string]any{}} }

func keyString(k any) string {
	switch v := k.(type) {
	case int64:
		return "i:" + strconv.FormatInt(v, 10)
	case int:
		return "i:" + strconv.Itoa(v)
	case string:
		// PHP turns numeric-string keys into ints.
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && strconv.FormatInt(n, 10) == v {
			return "i:" + v
		}
		return "s:" + v
	}
	return fmt.Sprint(k)
}

func normKey(k any) any {
	switch v := k.(type) {
	case int:
		return int64(v)
	case string:
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && strconv.FormatInt(n, 10) == v {
			return n
		}
	}
	return k
}

// Set puts key=value, keeping the key's position when it already exists.
func (a *Array) Set(k any, v any) *Array {
	k = normKey(k)
	ks := keyString(k)
	if _, ok := a.Values[ks]; !ok {
		a.Keys = append(a.Keys, k)
	}
	a.Values[ks] = v
	return a
}

// Append adds value under the next integer key.
func (a *Array) Append(v any) *Array {
	next := int64(0)
	for _, k := range a.Keys {
		if n, ok := k.(int64); ok && n >= next {
			next = n + 1
		}
	}
	return a.Set(next, v)
}

// Get is the value under key (nil when absent).
func (a *Array) Get(k any) any {
	if a == nil {
		return nil
	}
	return a.Values[keyString(normKey(k))]
}

// Has reports whether key exists.
func (a *Array) Has(k any) bool {
	if a == nil {
		return false
	}
	_, ok := a.Values[keyString(normKey(k))]
	return ok
}

// Delete removes key.
func (a *Array) Delete(k any) {
	if a == nil {
		return
	}
	ks := keyString(normKey(k))
	if _, ok := a.Values[ks]; !ok {
		return
	}
	delete(a.Values, ks)
	for i, key := range a.Keys {
		if keyString(key) == ks {
			a.Keys = append(a.Keys[:i], a.Keys[i+1:]...)
			break
		}
	}
}

// Len is the number of entries.
func (a *Array) Len() int {
	if a == nil {
		return 0
	}
	return len(a.Keys)
}

// String is the value under key as a string ("" when absent).
func (a *Array) String(k any) string { return ToString(a.Get(k)) }

// Arr is the value under key as an array (nil when absent or not an array).
func (a *Array) Arr(k any) *Array {
	v, _ := a.Get(k).(*Array)
	return v
}

// List is the array's values in order.
func (a *Array) List() []any {
	if a == nil {
		return nil
	}
	out := make([]any, 0, len(a.Keys))
	for _, k := range a.Keys {
		out = append(out, a.Values[keyString(k)])
	}
	return out
}

// StringList is the array's values as strings.
func (a *Array) StringList() []string {
	out := []string{}
	for _, v := range a.List() {
		out = append(out, ToString(v))
	}
	return out
}

// StringMap flattens a one-level array of scalars to a map.
func (a *Array) StringMap() map[string]string {
	out := map[string]string{}
	if a == nil {
		return out
	}
	for _, k := range a.Keys {
		v := a.Values[keyString(k)]
		if _, isArr := v.(*Array); isArr {
			continue
		}
		out[ToString(k)] = ToString(v)
	}
	return out
}

// FromStringMap builds an array from a map, keys sorted for stability.
func FromStringMap(m map[string]string) *Array {
	a := NewArray()
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		a.Set(k, m[k])
	}
	return a
}

// FromStrings builds a list array.
func FromStrings(values []string) *Array {
	a := NewArray()
	for _, v := range values {
		a.Append(v)
	}
	return a
}

// ToString renders a scalar the way PHP's string cast would.
func ToString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "1"
		}
		return ""
	case int64:
		return strconv.FormatInt(x, 10)
	case int:
		return strconv.Itoa(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return fmt.Sprint(v)
}

// ---------------------------------------------------------------- encoding

// Marshal serializes v. Supported: nil, bool, ints, floats, string, []string,
// []any, map[string]string, map[string]any (keys sorted) and *Array.
func Marshal(v any) string {
	var b strings.Builder
	write(&b, v)
	return b.String()
}

func write(b *strings.Builder, v any) {
	switch x := v.(type) {
	case nil:
		b.WriteString("N;")
	case bool:
		if x {
			b.WriteString("b:1;")
		} else {
			b.WriteString("b:0;")
		}
	case int:
		fmt.Fprintf(b, "i:%d;", x)
	case int64:
		fmt.Fprintf(b, "i:%d;", x)
	case uint64:
		fmt.Fprintf(b, "i:%d;", x)
	case float64:
		if x == math.Trunc(x) && math.Abs(x) < 1e15 {
			fmt.Fprintf(b, "d:%s;", strconv.FormatFloat(x, 'f', -1, 64))
		} else {
			fmt.Fprintf(b, "d:%s;", strconv.FormatFloat(x, 'g', 17, 64))
		}
	case string:
		fmt.Fprintf(b, "s:%d:\"%s\";", len(x), x)
	case []string:
		write(b, FromStrings(x))
	case []any:
		a := NewArray()
		for _, item := range x {
			a.Append(item)
		}
		write(b, a)
	case map[string]string:
		write(b, FromStringMap(x))
	case map[string]any:
		a := NewArray()
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			a.Set(k, x[k])
		}
		write(b, a)
	case *Array:
		fmt.Fprintf(b, "a:%d:{", x.Len())
		for _, k := range x.Keys {
			write(b, k)
			write(b, x.Values[keyString(k)])
		}
		b.WriteString("}")
	default:
		write(b, fmt.Sprint(v))
	}
}

// ---------------------------------------------------------------- decoding

// ErrSyntax is a malformed serialized string.
var ErrSyntax = errors.New("php: malformed serialized value")

// Unmarshal decodes a serialized value.
func Unmarshal(s string) (any, error) {
	d := decoder{s: s}
	v, err := d.value()
	if err != nil {
		return nil, err
	}
	return v, nil
}

// IsSerialized reports whether s looks like a serialized value (WordPress'
// is_serialized, simplified).
func IsSerialized(s string) bool {
	s = strings.TrimSpace(s)
	if s == "N;" {
		return true
	}
	if len(s) < 4 || s[1] != ':' {
		return false
	}
	switch s[0] {
	case 'a', 'O':
		return strings.HasSuffix(s, "}")
	case 's':
		return strings.HasSuffix(s, "\";")
	case 'i', 'd', 'b':
		return strings.HasSuffix(s, ";")
	}
	return false
}

// UnmarshalArray decodes s into an array, answering an empty one for a blank,
// unparsable or non-array value — the forgiving read WordPress code expects.
func UnmarshalArray(s string) *Array {
	if strings.TrimSpace(s) == "" {
		return NewArray()
	}
	v, err := Unmarshal(s)
	if err != nil {
		return NewArray()
	}
	if a, ok := v.(*Array); ok {
		return a
	}
	return NewArray()
}

type decoder struct {
	s   string
	pos int
}

func (d *decoder) readUntil(c byte) (string, error) {
	i := strings.IndexByte(d.s[d.pos:], c)
	if i < 0 {
		return "", ErrSyntax
	}
	out := d.s[d.pos : d.pos+i]
	d.pos += i + 1
	return out, nil
}

func (d *decoder) expect(c byte) error {
	if d.pos >= len(d.s) || d.s[d.pos] != c {
		return ErrSyntax
	}
	d.pos++
	return nil
}

func (d *decoder) value() (any, error) {
	if d.pos+1 >= len(d.s) {
		return nil, ErrSyntax
	}
	t := d.s[d.pos]
	if t == 'N' {
		d.pos++
		return nil, d.expect(';')
	}
	d.pos++
	if err := d.expect(':'); err != nil {
		return nil, err
	}
	switch t {
	case 'b':
		v, err := d.readUntil(';')
		return v == "1", err
	case 'i':
		v, err := d.readUntil(';')
		if err != nil {
			return nil, err
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, ErrSyntax
		}
		return n, nil
	case 'd':
		v, err := d.readUntil(';')
		if err != nil {
			return nil, err
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, ErrSyntax
		}
		return f, nil
	case 's':
		return d.str()
	case 'a':
		return d.array()
	case 'O':
		// Objects (rare in options WooCommerce writes): decoded as their
		// property array, class name dropped.
		if _, err := d.readUntil(':'); err != nil {
			return nil, err
		}
		if _, err := d.readUntil(':'); err != nil {
			return nil, err
		}
		d.pos-- // reuse array body parsing: "<count>:{...}"
		d.s = d.s[:d.pos] + d.s[d.pos:]
		if err := d.expect(':'); err != nil {
			return nil, err
		}
		return d.array()
	}
	return nil, ErrSyntax
}

func (d *decoder) str() (string, error) {
	lenStr, err := d.readUntil(':')
	if err != nil {
		return "", err
	}
	n, err := strconv.Atoi(lenStr)
	if err != nil || n < 0 {
		return "", ErrSyntax
	}
	if err := d.expect('"'); err != nil {
		return "", err
	}
	if d.pos+n > len(d.s) {
		return "", ErrSyntax
	}
	out := d.s[d.pos : d.pos+n]
	d.pos += n
	if err := d.expect('"'); err != nil {
		return "", err
	}
	if err := d.expect(';'); err != nil {
		return "", err
	}
	return out, nil
}

func (d *decoder) array() (*Array, error) {
	countStr, err := d.readUntil(':')
	if err != nil {
		return nil, err
	}
	count, err := strconv.Atoi(countStr)
	if err != nil || count < 0 {
		return nil, ErrSyntax
	}
	if err := d.expect('{'); err != nil {
		return nil, err
	}
	a := NewArray()
	for i := 0; i < count; i++ {
		k, err := d.value()
		if err != nil {
			return nil, err
		}
		v, err := d.value()
		if err != nil {
			return nil, err
		}
		a.Set(k, v)
	}
	if err := d.expect('}'); err != nil {
		return nil, err
	}
	return a, nil
}
