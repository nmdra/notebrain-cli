package mcpbridge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

func decodeResult(data []byte, command string) (map[string]any, string, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var object map[string]any
	if err := d.Decode(&object); err != nil || object == nil {
		return nil, "", fmt.Errorf("child returned an invalid JSON object")
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return nil, "", fmt.Errorf("child returned trailing JSON data")
	}
	if object["command"] != command {
		return nil, "", fmt.Errorf("child returned an unexpected command envelope")
	}
	stringField := func(k string) bool { _, ok := object[k].(string); return ok }
	countField := func(k string) bool {
		n, ok := object[k].(json.Number)
		if !ok {
			return false
		}
		v, e := strconv.ParseInt(string(n), 10, 64)
		return e == nil && v >= 0
	}
	arrayField := func(k string) bool {
		v, ok := object[k]
		if !ok {
			return false
		}
		if v == nil {
			return true
		}
		_, ok = v.([]any)
		return ok
	}
	valid := false
	switch command {
	case commandStats:
		valid = countField("notes") && countField("chunks") && countField("links")
	case commandGet:
		_, ok := object[fieldNote].(map[string]any)
		valid = stringField(fieldQuery) && ok
	case commandRefs:
		valid = stringField("note_slug") && stringField("title") && countField("total") && arrayField("refs")
	case "tags --list":
		valid = countField("total") && arrayField("tags")
	default:
		valid = stringField(fieldQuery) && countField("total") && arrayField("results")
	}
	if !valid {
		return nil, "", fmt.Errorf("child returned an invalid %s envelope", command)
	}
	compact, err := json.Marshal(object)
	if err != nil {
		return nil, "", fmt.Errorf("encode child result: %w", err)
	}
	return object, string(compact), nil
}
