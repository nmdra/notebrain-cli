package ort

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseShape parses a comma-separated shape string (for example: "1,384").
// All dimensions must be non-negative concrete sizes.
// Dynamic dimensions from model metadata (for example -1) are not accepted here.
func ParseShape(raw string) (Shape, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("shape string must not be empty")
	}

	parts := strings.Split(raw, ",")
	shape := make(Shape, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("empty dimension")
		}

		dim, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse dimension %q: %w", part, err)
		}
		if dim < 0 {
			return nil, fmt.Errorf("negative dimension %d (dynamic dimensions like -1 are not supported; provide concrete runtime sizes)", dim)
		}
		shape = append(shape, dim)
	}

	return shape, nil
}
