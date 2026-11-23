package mcpbridge

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
)

const (
	commandSearch = "search"
	commandTags   = "tags"
	commandStats  = "stats"
	fieldQueries  = "queries"
)

// Call is an allowlisted retrieval invocation. Args contains only positional values.
// Overrides contains explicit request flag values, with exclude-notes encoded as a JSON array.
type Call struct {
	Command   string
	Args      []string
	Overrides map[string]string
}

var commandFlags = map[string]string{
	commandSearch: "tag section has-tasks has-code with-pdf exclude-notes limit top-k include-text context-window min-score group-by-note",
	"get":         "meta head", "refs": "only-images only-pdf only-other only-external-links include-missing",
	"backlinks": "", "connections": "hops",
	"hidden":    "deep include-linked candidate-chunks limit include-text context-window min-score group-by-note",
	commandTags: "list shared for-note children min-shared limit include-text context-window min-score group-by-note",
	"boosted":   "seed boost with-pdf limit include-text context-window min-score group-by-note", commandStats: "",
}

// RetrievalFlags returns the sorted canonical config keys supported by this adapter.
func RetrievalFlags() []string {
	m := map[string]bool{}
	for _, s := range commandFlags {
		for k := range strings.FieldsSeq(s) {
			m[k] = true
		}
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func allowed(command, key string) bool {
	return slices.Contains(strings.Fields(commandFlags[command]), key)
}
func clone(m map[string]string) map[string]string {
	n := make(map[string]string, len(m))
	maps.Copy(n, m)
	return n
}

// EncodeExclusions serializes exact exclusion values for the private TOML profile.
func EncodeExclusions(v []string) string {
	if v == nil {
		v = []string{}
	}
	b, _ := json.Marshal(v)
	return string(b)
}
func makeCall(command string, input any, defaults map[string]string) (Call, map[string]string, error) {
	call := Call{Command: command, Overrides: map[string]string{}}
	raw, _ := json.Marshal(input)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	for k, v := range fields {
		if string(v) == "null" {
			return call, nil, fmt.Errorf("%s must not be null", k)
		}
		var ss []string
		if k == fieldQueries || k == "exclude_notes" {
			if err := json.Unmarshal(v, &ss); err != nil {
				return call, nil, fmt.Errorf("invalid %s", k)
			}
			maximum := 256
			if k == fieldQueries {
				maximum = 32
			}
			if len(ss) > maximum {
				return call, nil, fmt.Errorf("%s allows at most %d values", k, maximum)
			}
		}
		if k == fieldQueries {
			call.Args = ss
			continue
		}
		if k == "note" || k == "query" {
			var s string
			_ = json.Unmarshal(v, &s)
			call.Args = []string{s}
			continue
		}
		key := strings.ReplaceAll(k, "_", "-")
		if !allowed(command, key) {
			return call, nil, fmt.Errorf("unsupported field %s", k)
		}
		if k == "exclude_notes" {
			call.Overrides[key] = EncodeExclusions(ss)
		} else {
			var s string
			if json.Unmarshal(v, &s) == nil {
				call.Overrides[key] = s
			} else {
				call.Overrides[key] = string(v)
			}
		}
	}
	effective := clone(defaults)
	maps.Copy(effective, call.Overrides)
	if err := validateCall(call, effective); err != nil {
		return call, nil, err
	}
	return call, effective, nil
}
func boolValue(m map[string]string, k string) bool { v, _ := strconv.ParseBool(m[k]); return v }
func validateCall(c Call, e map[string]string) error {
	if _, ok := commandFlags[c.Command]; !ok {
		return fmt.Errorf("unsupported retrieval command")
	}
	for k := range c.Overrides {
		if !allowed(c.Command, k) {
			return fmt.Errorf("unsupported retrieval flag")
		}
	}
	if err := validateStrings(c, e); err != nil {
		return err
	}
	ranges := map[string][2]float64{"limit": {0, 100}, "top-k": {1, 3}, "hops": {1, 3}, "candidate-chunks": {1, 10}, "context-window": {0, 5}, "head": {0, 100}, "min-shared": {1, 100}, "boost": {0, 10}}
	for k, v := range e {
		if !allowed(c.Command, k) {
			continue
		}
		if strings.ContainsRune(v, 0) {
			return fmt.Errorf("invalid %s value", k)
		}
		if r, ok := ranges[k]; ok {
			n, err := strconv.ParseFloat(v, 64)
			if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < r[0] || n > r[1] || (k != "boost" && n != math.Trunc(n)) {
				return fmt.Errorf("%s must be in range %g..%g", k, r[0], r[1])
			}
		}
		if k == "min-score" {
			n, err := strconv.ParseFloat(v, 64)
			if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
				return fmt.Errorf("min-score must be finite")
			}
		}
	}
	nonblank := false
	for _, s := range c.Args {
		if strings.TrimSpace(s) != "" {
			nonblank = true
		}
	}
	switch c.Command {
	case commandSearch:
		if !nonblank && strings.TrimSpace(e["tag"]) == "" {
			return fmt.Errorf("search requires a nonblank query or tag")
		}
	case commandStats:
		if len(c.Args) != 0 {
			return fmt.Errorf("stats accepts no arguments")
		}
	case commandTags:
		if !boolValue(e, "list") && !nonblank {
			return fmt.Errorf("tags requires query unless list is true")
		}
	default:
		if !nonblank || len(c.Args) != 1 {
			return fmt.Errorf("%s requires one nonblank note or query", c.Command)
		}
	}
	if c.Command == "boosted" && strings.TrimSpace(e["seed"]) == "" {
		return fmt.Errorf("boosted requires seed")
	}
	return nil
}

// Count effective configured and explicit strings together before starting a child.
func validateStrings(c Call, e map[string]string) error {
	total := 0
	count := func(field string, values []string) error {
		for _, value := range values {
			if strings.ContainsRune(value, 0) {
				return fmt.Errorf("invalid %s value", field)
			}
			if len(value) > 16*1024 {
				return fmt.Errorf("%s exceeds 16 KiB per string", field)
			}
			total += len(value)
		}
		return nil
	}
	if err := count("positional input", c.Args); err != nil {
		return err
	}
	for _, key := range []string{"tag", "section", "seed"} {
		if allowed(c.Command, key) {
			if err := count(key, []string{e[key]}); err != nil {
				return err
			}
		}
	}
	if text, ok := e["exclude-notes"]; c.Command == commandSearch && ok {
		var values []string
		if err := json.Unmarshal([]byte(text), &values); err != nil || values == nil {
			return fmt.Errorf("exclude-notes requires a JSON array")
		}
		if len(values) > 256 {
			return fmt.Errorf("exclude-notes allows at most 256 values")
		}
		if err := count("exclude-notes", values); err != nil {
			return err
		}
	}
	if total > 64*1024 {
		return fmt.Errorf("input strings exceed 64 KiB")
	}
	return nil
}

func expectedCommand(c Call, e map[string]string) string {
	name := c.Command
	switch name {
	case "connections":
		hops := e["hops"]
		if hops == "" {
			hops = "2"
		}
		name += " --hops " + hops
	case "hidden":
		if boolValue(e, "deep") {
			name += " --deep"
		}
		if boolValue(e, "include-linked") {
			name += " --include-linked"
		}
	case commandTags:
		switch {
		case boolValue(e, "list"):
			name += " --list"
		case boolValue(e, "shared") || boolValue(e, "for-note"):
			name += " --shared"
		case boolValue(e, "children"):
			name += " --children"
		}
	}
	return name
}
