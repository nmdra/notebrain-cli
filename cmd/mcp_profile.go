package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"

	"github.com/alecthomas/kong"

	"github.com/nmdra/notebrain-cli/v2/internal/configfile"
	"github.com/nmdra/notebrain-cli/v2/internal/mcpbridge"
)

// captureMCPConfig records the same resolvers Kong applies, in load order.
// Values are resolved with the existing exact/normalized key precedence.
func captureMCPConfig(captured *[]kong.Resolver) kong.ConfigurationLoader {
	loader := configfile.IgnoreMissingFileLoader(configfile.TOMLResolver)
	return func(r io.Reader) (kong.Resolver, error) {
		resolver, err := loader(r)
		if err == nil && resolver != nil {
			*captured = append(*captured, resolver)
		}
		return resolver, err
	}
}

// privateProfileLoader checks the open file before reading it, not its path.
func privateProfileLoader(r io.Reader) (kong.Resolver, error) {
	f, ok := r.(*os.File)
	if !ok {
		return nil, fmt.Errorf("MCP child requires a private config file")
	}
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat private config: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return nil, fmt.Errorf("MCP child requires a private regular config file with mode 0600")
	}
	return configfile.IgnoreMissingFileLoader(configfile.TOMLResolver)(r)
}

func mcpRetrievalCommand(name string) bool {
	switch name {
	case groupSearch, groupGet, groupRefs, "backlinks", groupConn, groupHidden, groupTags, groupBoosted, groupStats:
		return true
	default:
		return false
	}
}

func mcpConfigSnapshot(resolvers []kong.Resolver, parser *kong.Kong) (map[string]string, error) {
	values := make(map[string]string)
	allowed := mcpbridge.RetrievalFlags()
	for _, node := range parser.Model.Children {
		if !mcpRetrievalCommand(node.Name) {
			continue
		}
		for _, flag := range node.Flags {
			if !slices.Contains(allowed, flag.Name) {
				continue
			}
			for _, resolver := range resolvers {
				value, err := resolver.Resolve(nil, nil, flag)
				if err != nil {
					return nil, fmt.Errorf("resolve %s: %w", flag.Name, err)
				}
				if value != nil {
					values[flag.Name] = mcpFlagValue(flag, value)
				}
			}
		}
	}
	return values, nil
}

// Reuse the same scalar and slice mappers as the CLI without mutating its flags.
// Invalid retrieval config does not prevent tool discovery. Affected calls reject it.
func mcpFlagValue(flag *kong.Flag, raw any) string {
	value := *flag.Value
	target := reflect.New(value.Target.Type()).Elem()
	if err := value.Parse(kong.Scan().PushTyped(raw, kong.FlagValueToken), target); err != nil {
		return "\x00"
	}
	if target.Kind() == reflect.Slice {
		if target.IsNil() {
			return "[]"
		}
		encoded, err := json.Marshal(target.Interface())
		if err != nil {
			return "\x00"
		}
		return string(encoded)
	}
	return fmt.Sprint(target.Interface())
}
