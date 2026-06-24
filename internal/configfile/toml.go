package configfile

import (
	"fmt"
	"io"
	"os"

	"github.com/alecthomas/kong"
	"github.com/pelletier/go-toml/v2"
)

// TOMLResolver is a kong.ConfigurationLoader that parses TOML files.
func TOMLResolver(r io.Reader) (kong.Resolver, error) {
	var parsed map[string]interface{}
	decoder := toml.NewDecoder(r)
	if err := decoder.Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}

	return kong.ResolverFunc(func(context *kong.Context, parent *kong.Path, flag *kong.Flag) (interface{}, error) {
		name := flag.Name

		val, ok := parsed[name]
		if ok {
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
