package store

import "encoding/json"

// marshalJSON renders a value for a TEXT column, defaulting to "{}" so the
// schema never holds a NULL where a decoder expects an object.
func marshalJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// unmarshalJSON decodes a TEXT column, tolerating empty values.
func unmarshalJSON(s string, out any) error {
	if s == "" {
		return nil
	}
	return json.Unmarshal([]byte(s), out)
}

// zeroDoc is the document a type's zero value encodes to, which is what a
// column holds until somebody sets it. It is "" for a type that does not encode.
func zeroDoc[T any]() string {
	b, err := json.Marshal(*new(T))
	if err != nil {
		return ""
	}
	return string(b)
}

// decodeFresh is unmarshalJSON for a struct still holding its zero value, which
// skips the decoder for the two documents that say nothing: an empty object, and
// what the zero value itself encodes to (zero, from zeroDoc). Either decodes to
// the zero value and to nothing else, and reading a pool, whose columns are
// nearly all one or the other, was most of what the decoder cost. Anything else,
// however alike, goes through it: a document with a space in it is not skipped.
//
// out must be a pointer to a struct that has not been decoded into. The zero
// value's encoding would reset one that had been, and an empty object decoded
// into a map is an empty map, which skipping it does not leave.
func decodeFresh[T any](doc, zero string, out *T) error {
	if doc == "" || doc == "{}" || doc == zero {
		return nil
	}
	return json.Unmarshal([]byte(doc), out)
}
