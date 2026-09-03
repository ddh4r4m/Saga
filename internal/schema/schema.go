// Package schema resolves schema ids of the form saga.<layer>.<thing>/<major>
// to the embedded files and validates records against them with a strict
// validator: an unknown field is a failure, not a warning (contracts
// section 11).
//
// The validator implements the JSON Schema subset the Saga schemas use
// (type, const, enum, pattern, minimum, minLength, required, properties,
// patternProperties, additionalProperties, items). Keeping it in the
// standard library keeps it off the hook cold-start path budget.
package schema

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/ddh4r4m/saga/schema"
)

// Registry maps schema ids to embedded files.
var Registry = map[string]string{
	"saga.trace/1":        "trace/1/envelope.json",
	"saga.trace.ledger/1": "trace/1/ledger.json",
	"saga.trace.pins/1":   "trace/1/pins.json",
	"saga.doctor/1":       "doctor/1/doctor.json",
}

// BodyFile returns the embedded body schema file for a trace event type.
func BodyFile(eventType string) string { return "trace/1/" + eventType + ".json" }

var (
	cacheMu sync.Mutex
	cache   = map[string]map[string]any{}
	reCache = map[string]*regexp.Regexp{}
)

// Load returns the parsed schema at the embedded file path.
func Load(file string) (map[string]any, error) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if s, ok := cache[file]; ok {
		return s, nil
	}
	raw, err := schemafs.FS.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("schema %s: %w", file, err)
	}
	var s map[string]any
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("schema %s: %w", file, err)
	}
	cache[file] = s
	return s, nil
}

// Raw returns the embedded schema bytes for a schema id.
func Raw(id string) ([]byte, error) {
	file, ok := Registry[id]
	if !ok {
		return nil, fmt.Errorf("unknown schema id %q", id)
	}
	return schemafs.FS.ReadFile(file)
}

// SplitID splits "saga.trace.ledger/1" into its name and major.
func SplitID(id string) (name string, major int, err error) {
	i := strings.LastIndexByte(id, '/')
	if i < 0 {
		return "", 0, fmt.Errorf("schema id %q has no major", id)
	}
	if _, err := fmt.Sscanf(id[i+1:], "%d", &major); err != nil {
		return "", 0, fmt.Errorf("schema id %q has a non-numeric major", id)
	}
	return id[:i], major, nil
}

// ValidateID validates a decoded JSON value against the schema registered
// under id.
func ValidateID(id string, v any) error {
	file, ok := Registry[id]
	if !ok {
		return fmt.Errorf("unknown schema id %q", id)
	}
	return ValidateFile(file, v)
}

// ValidateFile validates v against the embedded schema file.
func ValidateFile(file string, v any) error {
	s, err := Load(file)
	if err != nil {
		return err
	}
	return validate(s, v, "$")
}

// ValidateBytes decodes raw JSON (numbers as float64) and validates it
// against the schema registered under id.
func ValidateBytes(id string, raw []byte) error {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	return ValidateID(id, v)
}

// Normalize round-trips v through JSON so struct values validate like the
// decoded documents they will become.
func Normalize(v any) (any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func typeOf(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case float64:
		if t == math.Trunc(t) {
			return "integer"
		}
		return "number"
	case json.Number:
		if _, err := t.Int64(); err == nil {
			return "integer"
		}
		return "number"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	}
	return "unknown"
}

func typeMatches(want string, v any) bool {
	got := typeOf(v)
	if want == "number" && got == "integer" {
		return true
	}
	return want == got
}

func numberOf(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	}
	return 0, false
}

func compile(p string) (*regexp.Regexp, error) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if re, ok := reCache[p]; ok {
		return re, nil
	}
	re, err := regexp.Compile(p)
	if err != nil {
		return nil, err
	}
	reCache[p] = re
	return re, nil
}

func validate(s map[string]any, v any, path string) error {
	if t, ok := s["type"]; ok {
		var types []string
		switch tt := t.(type) {
		case string:
			types = []string{tt}
		case []any:
			for _, x := range tt {
				types = append(types, x.(string))
			}
		}
		matched := false
		for _, want := range types {
			if typeMatches(want, v) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%s: type %s, want %s", path, typeOf(v), strings.Join(types, "|"))
		}
	}
	if c, ok := s["const"]; ok {
		if !equalJSON(c, v) {
			return fmt.Errorf("%s: value %v, want const %v", path, v, c)
		}
	}
	if e, ok := s["enum"]; ok {
		found := false
		for _, x := range e.([]any) {
			if equalJSON(x, v) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%s: value %v not in enum", path, v)
		}
	}
	switch val := v.(type) {
	case string:
		if p, ok := s["pattern"].(string); ok {
			re, err := compile(p)
			if err != nil {
				return fmt.Errorf("%s: bad pattern %q: %w", path, p, err)
			}
			if !re.MatchString(val) {
				return fmt.Errorf("%s: %q does not match %s", path, val, p)
			}
		}
		if ml, ok := numberOf(s["minLength"]); ok && float64(len(val)) < ml {
			return fmt.Errorf("%s: shorter than %v", path, ml)
		}
	case float64, json.Number:
		if m, ok := numberOf(s["minimum"]); ok {
			f, _ := numberOf(val)
			if f < m {
				return fmt.Errorf("%s: %v below minimum %v", path, f, m)
			}
		}
	case map[string]any:
		if req, ok := s["required"].([]any); ok {
			for _, r := range req {
				if _, present := val[r.(string)]; !present {
					return fmt.Errorf("%s: missing required field %q", path, r)
				}
			}
		}
		props, _ := s["properties"].(map[string]any)
		patterns, _ := s["patternProperties"].(map[string]any)
		additional, hasAdditional := s["additionalProperties"]
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			child := path + "." + k
			if ps, ok := props[k].(map[string]any); ok {
				if err := validate(ps, val[k], child); err != nil {
					return err
				}
				continue
			}
			matchedPattern := false
			for p, ps := range patterns {
				re, err := compile(p)
				if err != nil {
					return fmt.Errorf("%s: bad pattern %q: %w", path, p, err)
				}
				if re.MatchString(k) {
					matchedPattern = true
					if err := validate(ps.(map[string]any), val[k], child); err != nil {
						return err
					}
				}
			}
			if matchedPattern {
				continue
			}
			if hasAdditional {
				switch a := additional.(type) {
				case bool:
					if !a {
						return fmt.Errorf("%s: unknown field %q", path, k)
					}
				case map[string]any:
					if err := validate(a, val[k], child); err != nil {
						return err
					}
				}
			}
		}
	case []any:
		if items, ok := s["items"].(map[string]any); ok {
			for i, e := range val {
				if err := validate(items, e, fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func equalJSON(a, b any) bool {
	if fa, ok := numberOf(a); ok {
		fb, ok := numberOf(b)
		return ok && fa == fb
	}
	return a == b
}
