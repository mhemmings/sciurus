package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

const maxDocumentBytes = 16 << 20

// Decode reads exactly one versioned document. Duplicate keys and unknown fields
// are errors rather than accepting ambiguous AI edits or silently losing data.
func Decode(data []byte) (*Document, error) {
	if len(data) > maxDocumentBytes {
		return nil, fmt.Errorf("model exceeds the 16 MiB limit")
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("model must contain valid UTF-8")
	}
	// encoding/json replaces unpaired escaped surrogates, which would silently
	// alter identifiers. Reject them before the standard decoder can repair data.
	for i := 0; i < len(data); i++ {
		if data[i] != '\\' || i+1 >= len(data) {
			continue
		}
		i++
		if data[i] != 'u' || i+4 >= len(data) {
			continue
		}
		value, parseErr := strconv.ParseUint(string(data[i+1:i+5]), 16, 16)
		if parseErr != nil {
			continue
		}
		i += 4
		if value >= 0xdc00 && value <= 0xdfff {
			return nil, fmt.Errorf("unpaired Unicode surrogate in JSON string")
		}
		if value >= 0xd800 && value <= 0xdbff {
			if i+6 >= len(data) || string(data[i+1:i+3]) != "\\u" {
				return nil, fmt.Errorf("unpaired Unicode surrogate in JSON string")
			}
			low, lowErr := strconv.ParseUint(string(data[i+3:i+7]), 16, 16)
			if lowErr != nil || low < 0xdc00 || low > 0xdfff {
				return nil, fmt.Errorf("unpaired Unicode surrogate in JSON string")
			}
			i += 6
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	err := inspectJSON(decoder, 0)
	if err != nil {
		return nil, err
	}
	_, err = decoder.Token()
	if err != io.EOF {
		return nil, fmt.Errorf("expected one JSON document")
	}
	err = inspectFields(data, reflect.TypeFor[Document](), "model")
	if err != nil {
		return nil, err
	}
	var document *Document
	err = json.Unmarshal(data, &document)
	if err != nil {
		return nil, fmt.Errorf("decode model: %w", err)
	}
	err = Validate(document)
	if err != nil {
		return nil, err
	}
	return document, nil
}

// inspectFields makes property spelling exact. encoding/json otherwise accepts
// case-insensitive aliases, even with DisallowUnknownFields enabled.
func inspectFields(data []byte, shape reflect.Type, path string) error {
	nullable := shape.Kind() == reflect.Pointer
	if shape.Kind() == reflect.Pointer {
		shape = shape.Elem()
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		if nullable {
			return nil
		}
		return fmt.Errorf("%s cannot be null; omit optional properties instead", path)
	}
	switch shape.Kind() {
	case reflect.Struct:
		var fields map[string]json.RawMessage
		err := json.Unmarshal(data, &fields)
		if err != nil {
			return err
		}
		known := map[string]reflect.Type{}
		required := []string{}
		for field := range shape.Fields() {
			tag := strings.Split(field.Tag.Get("json"), ",")
			known[tag[0]] = field.Type
			if len(tag) == 1 {
				required = append(required, tag[0])
			}
		}
		for _, name := range required {
			if _, present := fields[name]; !present {
				return fmt.Errorf("%s: required field %q is missing", path, name)
			}
		}
		for name, value := range fields {
			field, ok := known[name]
			if !ok {
				return fmt.Errorf("%s: unknown field %q (property names are case-sensitive)", path, name)
			}
			err = inspectFields(value, field, path+"."+name)
			if err != nil {
				return err
			}
		}
	case reflect.Slice:
		var elements []json.RawMessage
		err := json.Unmarshal(data, &elements)
		if err != nil {
			return err
		}
		for i, value := range elements {
			err = inspectFields(value, shape.Elem(), fmt.Sprintf("%s[%d]", path, i))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// inspectJSON walks tokens before decoding so repeated object keys cannot be
// hidden by encoding/json's last-value-wins behavior. Depth is bounded explicitly.
func inspectJSON(decoder *json.Decoder, depth int) error {
	if depth > 64 {
		return fmt.Errorf("JSON nesting exceeds 64 levels")
	}
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if delimiter != '[' && delimiter != '{' {
		return fmt.Errorf("unexpected JSON delimiter")
	}
	seen := map[string]bool{}
	for decoder.More() {
		if delimiter == '{' {
			keyToken, keyErr := decoder.Token()
			if keyErr != nil {
				return keyErr
			}
			key, valid := keyToken.(string)
			if !valid {
				return fmt.Errorf("expected object key")
			}
			if seen[key] {
				return fmt.Errorf("duplicate JSON key %q", key)
			}
			seen[key] = true
		}
		err = inspectJSON(decoder, depth+1)
		if err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}

// Encode returns reproducibly formatted UTF-8 JSON with a trailing newline.
// It never sorts ordered collections or silently repairs invalid documents.
func Encode(document *Document) ([]byte, error) {
	err := Validate(document)
	if err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(document)
	if err != nil {
		return nil, err
	}
	if buffer.Len() > maxDocumentBytes {
		return nil, fmt.Errorf("model exceeds the 16 MiB limit")
	}
	return buffer.Bytes(), nil
}
