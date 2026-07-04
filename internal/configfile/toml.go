package configfile

import (
	"fmt"
	"io"
	"os"
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

// TOMLResolver is a kong.ConfigurationLoader that parses TOML files.
func TOMLResolver(r io.Reader) (kong.Resolver, error) {
	var parsed map[string]interface{}
	decoder := toml.NewDecoder(r)
	if err := decoder.Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}

	normalized := make(map[string]interface{}, len(parsed))
	for k, v := range parsed {
		normalized[normalizeKey(k)] = v
	}

	return kong.ResolverFunc(func(context *kong.Context, parent *kong.Path, flag *kong.Flag) (interface{}, error) {
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
				return kong.ResolverFunc(func(*kong.Context, *kong.Path, *kong.Flag) (interface{}, error) { return nil, nil }), nil
			}
		}
		return loader(r)
	}
}
