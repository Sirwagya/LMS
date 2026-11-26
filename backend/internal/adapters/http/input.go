package httpadapter

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

const maxAttachment = 10 << 20

type fields struct {
	values map[string]json.RawMessage
	err    error
}

func read(w http.ResponseWriter, r *http.Request) *fields {
	return readBody(w, r, 16<<20)
}

func readBody(w http.ResponseWriter, r *http.Request, maxBytes int64) *fields {
	f := &fields{}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes))
	err := d.Decode(&f.values)
	if err == nil {
		var extra any
		if next := d.Decode(&extra); next != io.EOF {
			err = next
			if err == nil {
				err = errors.New("extra JSON value")
			}
		}
	}
	var large *http.MaxBytesError
	switch {
	case errors.As(err, &large):
		f.err = problem(413, "Request body is too large.")
	case err != nil && err != io.EOF:
		f.err = invalid("Invalid JSON.")
	}
	return f
}
func (f *fields) fail(err error) {
	if f.err == nil {
		f.err = err
	}
}
func (f *fields) text(key, label string, max int) string {
	var value string
	err := json.Unmarshal(f.values[key], &value)
	if err != nil || strings.TrimSpace(value) == "" || textLength(value) > max {
		f.fail(invalid(fmt.Sprintf("Valid %s required (max %d characters).", label, max)))
	}
	return strings.TrimSpace(value)
}
func textLength(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}
func (f *fields) optional(key string) string {
	var value string
	_ = json.Unmarshal(f.values[key], &value)
	return value
}
func (f *fields) id(key string) string {
	if v := f.optional(key); v != "" {
		return v
	}
	v, err := strconv.ParseInt(string(f.values[key]), 10, 64)
	if err != nil {
		return ""
	}
	return strconv.FormatInt(v, 10)
}
func (f *fields) integer(key, label string, min, max int) int {
	v, err := strconv.ParseFloat(string(f.values[key]), 64)
	if err != nil || math.IsNaN(v) || v < float64(min) || v > float64(max) || math.Trunc(v) != v {
		f.fail(invalid("Invalid " + label + "."))
		return 0
	}
	return int(v)
}
func (f *fields) date(key, message string) string {
	v := f.optional(key)
	if _, err := time.Parse(time.DateOnly, v); err != nil {
		f.fail(invalid(message))
	}
	return v
}
func (f *fields) flag(key string) bool {
	var value bool
	_ = json.Unmarshal(f.values[key], &value)
	return value
}
func (f *fields) bytes(key, tooLarge string) []byte {
	if f.err != nil {
		return nil
	}
	value := f.optional(key)
	if len(value) > base64.StdEncoding.EncodedLen(maxAttachment)+2 {
		f.fail(problem(413, tooLarge))
		return nil
	}
	bytes, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		bytes, err = base64.RawStdEncoding.DecodeString(value)
	}
	if err != nil {
		f.fail(invalid("Invalid base64 file contents."))
		return nil
	}
	if len(bytes) > maxAttachment {
		f.fail(problem(413, tooLarge))
		return nil
	}
	return bytes
}
func intID(value string) int64 { id, _ := strconv.ParseInt(value, 10, 64); return id }
func items[T any](list []T) []T {
	if list == nil {
		return []T{}
	}
	return list
}
