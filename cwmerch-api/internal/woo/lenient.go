package woo

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
)

// Product and variation inputs keep WooCommerce's text fields as strings
// (prices, SKU, weight, dimensions...), but clients often send the numbers
// they read back (the API answers prices as numbers): a JSON number is
// accepted for any string field and kept as its text ("30", "12.5").

var stringFieldCache sync.Map // reflect.Type -> map[string]bool (JSON keys of string / *string fields)

func stringFields(t reflect.Type) map[string]bool {
	if v, ok := stringFieldCache.Load(t); ok {
		return v.(map[string]bool)
	}
	keys := map[string]bool{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		ft := f.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft.Kind() != reflect.String {
			continue
		}
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		keys[name] = true
	}
	stringFieldCache.Store(t, keys)
	return keys
}

// lenientStrings rewrites the JSON object b so numbers given for the string
// fields of t become JSON strings.
func lenientStrings(b []byte, t reflect.Type) ([]byte, error) {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return b, nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return nil, err
	}
	keys := stringFields(t)
	changed := false
	for k, v := range raw {
		v = bytes.TrimSpace(v)
		if !keys[k] || len(v) == 0 || !(v[0] == '-' || (v[0] >= '0' && v[0] <= '9')) {
			continue
		}
		var n json.Number
		if json.Unmarshal(v, &n) != nil {
			continue
		}
		quoted, _ := json.Marshal(n.String())
		raw[k] = quoted
		changed = true
	}
	if !changed {
		return b, nil
	}
	return json.Marshal(raw)
}

// UnmarshalJSON accepts numbers for the string fields (prices, SKU...).
func (p *ProductInput) UnmarshalJSON(b []byte) error {
	type plain ProductInput
	fixed, err := lenientStrings(b, reflect.TypeOf(plain{}))
	if err != nil {
		return err
	}
	return json.Unmarshal(fixed, (*plain)(p))
}

// UnmarshalJSON accepts numbers for the string fields (prices, SKU...).
func (v *VariationInput) UnmarshalJSON(b []byte) error {
	type plain VariationInput
	fixed, err := lenientStrings(b, reflect.TypeOf(plain{}))
	if err != nil {
		return err
	}
	return json.Unmarshal(fixed, (*plain)(v))
}
