// Package jsonx reads and writes JSON objects WITHOUT reordering their keys.
//
// It exists for one file: the user's ~/.claude/settings.json. That file belongs
// to the user, carries their own settings, and `ctx bootstrap` only ever adds a
// missing hook entry to it. Go's encoding/json unmarshals objects into maps,
// and maps have no order — round-tripping through one would rewrite the whole
// file with alphabetised keys. A setup command that scrambles the file it was
// asked to touch is not a setup command.
//
// Output matches Python's json.dumps(..., ensure_ascii=False, indent=2), which
// is what the canonical implementation writes.
package jsonx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// Object is a JSON object that remembers the order its keys arrived in.
type Object struct {
	keys   []string
	values map[string]any
}

// NewObject returns an empty object.
func NewObject() *Object {
	return &Object{values: map[string]any{}}
}

// Get returns the value for a key.
func (o *Object) Get(key string) (any, bool) {
	if o == nil {
		return nil, false
	}
	v, ok := o.values[key]
	return v, ok
}

// Set stores a value, appending the key only if it is new — so updating an
// existing entry leaves it where the user put it.
func (o *Object) Set(key string, value any) {
	if o.values == nil {
		o.values = map[string]any{}
	}
	if _, exists := o.values[key]; !exists {
		o.keys = append(o.keys, key)
	}
	o.values[key] = value
}

// Object returns a nested object under key, creating it when absent. Mirrors
// Python's setdefault(key, {}) chain.
func (o *Object) Object(key string) *Object {
	if v, ok := o.values[key]; ok {
		if nested, ok := v.(*Object); ok {
			return nested
		}
	}
	nested := NewObject()
	o.Set(key, nested)
	return nested
}

// Array returns a list under key, creating it when absent.
func (o *Object) Array(key string) []any {
	if v, ok := o.values[key]; ok {
		if list, ok := v.([]any); ok {
			return list
		}
	}
	list := []any{}
	o.Set(key, list)
	return list
}

// Parse reads JSON, preserving object key order throughout.
func Parse(data []byte) (*Object, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber() // keep 20 as "20", not 2e+01
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, fmt.Errorf("kok bir JSON nesnesi degil")
	}
	obj, err := parseObject(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF && err != nil {
		return nil, err
	}
	return obj, nil
}

func parseObject(dec *json.Decoder) (*Object, error) {
	obj := NewObject()
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("nesne anahtari string degil")
		}
		value, err := parseValue(dec)
		if err != nil {
			return nil, err
		}
		obj.Set(key, value)
	}
	if _, err := dec.Token(); err != nil { // closing }
		return nil, err
	}
	return obj, nil
}

func parseValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := tok.(json.Delim); ok {
		switch delim {
		case '{':
			return parseObject(dec)
		case '[':
			list := []any{}
			for dec.More() {
				item, err := parseValue(dec)
				if err != nil {
					return nil, err
				}
				list = append(list, item)
			}
			if _, err := dec.Token(); err != nil { // closing ]
				return nil, err
			}
			return list, nil
		}
		return nil, fmt.Errorf("beklenmeyen sinirlayici: %v", delim)
	}
	return tok, nil
}

// Marshal renders the object with two-space indentation — Python's
// json.dumps(..., ensure_ascii=False, indent=2).
func Marshal(obj *Object) (string, error) {
	var b strings.Builder
	if err := writeValue(&b, obj, 0); err != nil {
		return "", err
	}
	return b.String(), nil
}

// MarshalCompact renders on one line — Python's json.dumps(...) with its
// DEFAULTS, which is what the hook scripts use: separators ", " and ": ", and
// ensure_ascii=True, so every non-ASCII character becomes \uXXXX.
//
// The escaping is not cosmetic here: the hook output is compared byte for byte
// against the Python hook it replaces, and the memory map it carries is full of
// Turkish characters.
func MarshalCompact(obj *Object) (string, error) {
	var b strings.Builder
	if err := writeCompact(&b, obj); err != nil {
		return "", err
	}
	return b.String(), nil
}

func writeCompact(b *strings.Builder, value any) error {
	switch v := value.(type) {
	case *Object:
		b.WriteString("{")
		for i, key := range v.keys {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(escapeASCII(encodeString(key)))
			b.WriteString(": ")
			if err := writeCompact(b, v.values[key]); err != nil {
				return err
			}
		}
		b.WriteString("}")
	case []any:
		b.WriteString("[")
		for i, item := range v {
			if i > 0 {
				b.WriteString(", ")
			}
			if err := writeCompact(b, item); err != nil {
				return err
			}
		}
		b.WriteString("]")
	case string:
		b.WriteString(escapeASCII(encodeString(v)))
	default:
		var inner strings.Builder
		if err := writeValue(&inner, value, 0); err != nil {
			return err
		}
		b.WriteString(inner.String())
	}
	return nil
}

// escapeASCII replaces every non-ASCII rune in an already-quoted JSON string
// with its \uXXXX form, using surrogate pairs above the BMP — exactly what
// Python's ensure_ascii=True produces.
func escapeASCII(quoted string) string {
	var b strings.Builder
	for _, r := range quoted {
		if r < utf8.RuneSelf {
			b.WriteRune(r)
			continue
		}
		if r > 0xFFFF {
			r -= 0x10000
			fmt.Fprintf(&b, "\\u%04x\\u%04x", 0xD800+(r>>10), 0xDC00+(r&0x3FF))
			continue
		}
		fmt.Fprintf(&b, "\\u%04x", r)
	}
	return b.String()
}

func writeValue(b *strings.Builder, value any, depth int) error {
	pad := strings.Repeat("  ", depth)
	inner := strings.Repeat("  ", depth+1)

	switch v := value.(type) {
	case *Object:
		if v == nil || len(v.keys) == 0 {
			b.WriteString("{}")
			return nil
		}
		b.WriteString("{\n")
		for i, key := range v.keys {
			b.WriteString(inner)
			b.WriteString(encodeString(key))
			b.WriteString(": ")
			if err := writeValue(b, v.values[key], depth+1); err != nil {
				return err
			}
			if i < len(v.keys)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(pad)
		b.WriteString("}")
	case []any:
		if len(v) == 0 {
			b.WriteString("[]")
			return nil
		}
		b.WriteString("[\n")
		for i, item := range v {
			b.WriteString(inner)
			if err := writeValue(b, item, depth+1); err != nil {
				return err
			}
			if i < len(v)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(pad)
		b.WriteString("]")
	case string:
		b.WriteString(encodeString(v))
	case json.Number:
		b.WriteString(v.String())
	case int:
		fmt.Fprintf(b, "%d", v)
	case bool:
		if v {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case nil:
		b.WriteString("null")
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		b.Write(raw)
	}
	return nil
}

// encodeString quotes a string the way Python does with ensure_ascii=False:
// non-ASCII stays literal, and HTML characters are NOT escaped.
func encodeString(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return `""`
	}
	return strings.TrimRight(buf.String(), "\n")
}
