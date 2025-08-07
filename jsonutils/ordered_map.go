// Copyright 2015 go-swagger maintainers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package jsonutils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
)

// JSONMapSlice represents a JSON object, with the order of keys maintained.
type JSONMapSlice []JSONMapItem

// MarshalJSON renders a [JSONMapSlice] as JSON bytes, preserving the order of keys.
func (s JSONMapSlice) MarshalJSON() ([]byte, error) {
	w := &jsonBuffer{
		buffer: make([]byte, 0),
	}
	s.JSONmarshal(w)

	return w.buffer, w.err
}

type jsonBuffer struct {
	buffer []byte
	err    error
}

type jsonDecoder struct {
	decoder      *json.Decoder
	currentToken json.Token
	err          error
}

func (jb *jsonBuffer) appendRawByte(b byte) {
	jb.buffer = append(jb.buffer, b)
}

func (jb *jsonBuffer) appendByteSlice(b []byte) {
	jb.buffer = append(jb.buffer, b...)
}

func (jb *jsonBuffer) appendString(b []byte) {
	jb.buffer = append(jb.buffer, '"')
	jb.buffer = append(jb.buffer, b...)
	jb.buffer = append(jb.buffer, '"')
}

func (s JSONMapSlice) JSONmarshal(w *jsonBuffer) {
	if s == nil {
		w.appendByteSlice([]byte("null"))
		return
	}

	w.appendRawByte('{')

	if len(s) == 0 {
		w.appendRawByte('}')

		return
	}

	s[0].JSONmarshal(w)

	for i := 1; i < len(s); i++ {
		w.appendRawByte(',')
		s[i].JSONmarshal(w)
	}

	w.appendRawByte('}')
}

// UnmarshalJSON builds a [JSONMapSlice] from JSON bytes, preserving the order of keys.
//
// Inner objects are unmarshaled as [JSONMapSlice] slices and not map[string]any.
func (s *JSONMapSlice) UnmarshalJSON(data []byte) error {
	d := &jsonDecoder{
		decoder: json.NewDecoder(bytes.NewReader(data)),
	}
	t, err := d.decoder.Token()
	if err == io.EOF {
		return nil
	}

	delim, ok := t.(json.Delim)
	if !ok {
		return fmt.Errorf("expected delimeter")
	}
	if delim != '{' {
		return fmt.Errorf("expected '{' delimeter, got %s", delim)
	}

	s.JSONunmarshal(data, d)
	return d.err
}

// JSONunmarshal builds a [JSONMapSlice] from JSON bytes, using CustomJSON
func (s *JSONMapSlice) JSONunmarshal(data []byte, d *jsonDecoder) {

	result := make(JSONMapSlice, 0)

	for {
		t, err := d.decoder.Token()
		if del, ok := t.(json.Delim); ok && del == '}' {
			break
		}
		if err == io.EOF {
			break
		}
		d.currentToken = t
		var mi JSONMapItem
		mi.UnmarshalCustomJSON(d, data)

		result = append(result, mi)
	}

	*s = result
}

// JSONMapItem represents the value of a key in a JSON object held by [JSONMapSlice].
//
// Notice that JSONMapItem should not be marshaled to or unmarshaled from JSON directly,
// use this type as part of a [JSONMapSlice] when dealing with JSON bytes.
type JSONMapItem struct {
	Key   string
	Value any
}

// MarshalCustomJSON renders a [JSONMapItem] as JSON bytes, using CustomJSON
func (s JSONMapItem) JSONmarshal(jb *jsonBuffer) {
	jb.appendString([]byte(s.Key))
	jb.appendRawByte(':')
	jsonRes, err := WriteJSON(s.Value)
	if err != nil {
		fmt.Println(s.Value)
		log.Fatal(err)
	}
	jb.appendByteSlice(jsonRes)
}

// UnmarshalCustomJSON builds a [JSONMapItem] from JSON bytes, using CustomJSON
func (s *JSONMapItem) UnmarshalCustomJSON(d *jsonDecoder, data []byte) {
	var key string
	var value any
	offset := d.decoder.InputOffset()
	if data[offset] == ':' {
		key = d.currentToken.(string)
	} else {
		return
	}
	t, err := d.decoder.Token()
	if err != nil {
		d.err = err
		return
	}

	d.currentToken = t
	value = s.asInterface(d, data)

	s.Key = key
	s.Value = value
}

// asInterface is very much like [jlexer.Lexer.Interface], but unmarshals an object
// into a [JSONMapSlice], not a map[string]any.
//
// We have to force parsing errors somehow, since [jlexer.Lexer] doesn't let us
// set a parsing error directly.
func (s *JSONMapItem) asInterface(d *jsonDecoder, data []byte) any {
	switch n := d.currentToken.(type) {
	case json.Delim:
		converted := string(n)
		if converted == "{" {
			ret := make(JSONMapSlice, 0)
			ret.JSONunmarshal(data, d)
			return ret
		} else if converted == "[" {
			ret := []any{}
			for d.decoder.More() {
				t, err := d.decoder.Token()
				if err != nil {
					d.err = err
					return nil
				}
				d.currentToken = t
				ret = append(ret, s.asInterface(d, data))
			}
			// advance
			_, err := d.decoder.Token()
			if err != nil {
				d.err = err
				return nil
			}
			return ret
		}
	case string:
		return n
	case int64:
		// determine if we may use an integer type
		strN := strconv.FormatInt(n, 10)
		if strings.ContainsRune(strN, '.') {
			f, _ := strconv.ParseFloat(strN, 64)
			return f
		} else {
			i, _ := strconv.ParseInt(strN, 10, 64)
			return i
		}
	default:
		return n
	}

	return nil
}
