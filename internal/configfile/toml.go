package configfile

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/pelletier/go-toml/v2"
)

// normalizeKey strips hyphens and underscores and converts to lowercase
// so that snake_case, kebab-case, and PascalCase keys match interchangeably.
func normalizeKey(s string) string {
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	return strings.ToLower(s)
}

// buildNormalizedKeys flattens parsed TOML keys through normalizeKey in a
// deterministic order. If two keys collide (e.g. "show-tags" and "show_tags")
// the lexicographically first one wins and the duplicates are dropped with a
// warning; without the sort, map iteration order would decide the winner
// randomly per run.
func buildNormalizedKeys(parsed map[string]any) map[string]any {
	keys := make([]string, 0, len(parsed))
	for k := range parsed {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	normalized := make(map[string]any, len(parsed))
	for _, k := range keys {
		nk := normalizeKey(k)
		if _, dup := normalized[nk]; dup {
			slog.Warn("ambiguous config keys normalize to the same flag; keeping the lexicographically first", "key", k)
			continue
		}
		normalized[nk] = parsed[k]
	}
	return normalized
}

// TOMLResolver is a kong.ConfigurationLoader that parses TOML files.
func TOMLResolver(r io.Reader) (kong.Resolver, error) {
	var parsed map[string]any
	decoder := toml.NewDecoder(r)
	if err := decoder.Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}

	normalized := buildNormalizedKeys(parsed)

	return kong.ResolverFunc(func(_ *kong.Context, _ *kong.Path, flag *kong.Flag) (any, error) {
		name := flag.Name

		if val, ok := parsed[name]; ok {
			return val, nil
		}
		if val, ok := normalized[normalizeKey(name)]; ok {
			return val, nil
		}

		return nil, nil
	}), nil
}

// IgnoreMissingFileLoader wraps a loader so that it silently ignores if the file does not exist.
func IgnoreMissingFileLoader(loader kong.ConfigurationLoader) kong.ConfigurationLoader {
	return func(r io.Reader) (kong.Resolver, error) {
		if f, ok := r.(*os.File); ok {
			stat, err := f.Stat()
			// If we can't stat it, or it's empty, just return an empty resolver
			if err != nil || stat.Size() == 0 {
				return kong.ResolverFunc(func(*kong.Context, *kong.Path, *kong.Flag) (any, error) { return nil, nil }), nil
			}
		}
		return loader(r)
	}
}
