package configfile

import (
	"bytes"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/pelletier/go-toml/v2"
)

type ConfigStruct struct {
	StringField  string `kong:"name='string-field'"`
	IntField     int    `kong:"name='int-field'"`
	BoolField    bool   `kong:"name='bool-field'"`
	DefaultField string `kong:"name='default-field',default='def'"`
}

func TestTOMLResolver(t *testing.T) {
	tomlData := []byte(`
string-field = "hello"
int-field = 42
bool-field = true
`)

	resolver, err := TOMLResolver(bytes.NewReader(tomlData))
	if err != nil {
		t.Fatalf("TOMLResolver failed: %v", err)
	}

	var cli ConfigStruct
	parser, err := kong.New(&cli, kong.Resolvers(resolver))
	if err != nil {
		t.Fatalf("kong.New failed: %v", err)
	}

	_, err = parser.Parse([]string{})
	if err != nil {
		t.Fatalf("parser.Parse failed: %v", err)
	}

	if cli.StringField != "hello" {
		t.Errorf("Expected string-field 'hello', got %q", cli.StringField)
	}
	if cli.IntField != 42 {
		t.Errorf("Expected int-field 42, got %d", cli.IntField)
	}
	if cli.BoolField != true {
		t.Errorf("Expected bool-field true, got %v", cli.BoolField)
	}
	if cli.DefaultField != "def" {
		t.Errorf("Expected default-field 'def', got %q", cli.DefaultField)
	}
}

func TestTOMLResolver_NormalizedKeys(t *testing.T) {
	tomlData := []byte(`
string_field = "snake"
int_field = 99
bool_field = false
`)

	resolver, err := TOMLResolver(bytes.NewReader(tomlData))
	if err != nil {
		t.Fatalf("TOMLResolver failed: %v", err)
	}

	var cli ConfigStruct
	parser, err := kong.New(&cli, kong.Resolvers(resolver))
	if err != nil {
		t.Fatalf("kong.New failed: %v", err)
	}

	_, err = parser.Parse([]string{})
	if err != nil {
		t.Fatalf("parser.Parse failed: %v", err)
	}

	if cli.StringField != "snake" {
		t.Errorf("Expected string-field 'snake', got %q", cli.StringField)
	}
	if cli.IntField != 99 {
		t.Errorf("Expected int-field 99, got %d", cli.IntField)
	}
	if cli.BoolField != false {
		t.Errorf("Expected bool-field false, got %v", cli.BoolField)
	}
}

type CoreGlobals struct {
	ChromaPath    string  `help:"path to ChromaDB persistent storage" default:"~/.notebrain/chroma"`
	VaultPath     string  `name:"vault-path" help:"Obsidian vault path"`
	ContextWindow int     `name:"context-window" default:"0"`
	MinScore      float64 `default:"0"`
	Debug         bool    `name:"debug" default:"false"`
	LogLevel      string  `name:"log-level" default:"info"`
	SkipPhantom   bool    `name:"skip-phantom" default:"true"`
	ShowTags      bool    `name:"show-tags" default:"false"`
}

func TestTOMLResolver_StrictNoHTTP(t *testing.T) {
	tomlData := []byte(`
chroma-path = "/tmp/custom-chroma"
vault_path = "/tmp/my-vault"
context_window = 2
min_score = 0.75
debug = true
`)

	resolver, err := TOMLResolver(bytes.NewReader(tomlData))
	if err != nil {
		t.Fatalf("TOMLResolver failed: %v", err)
	}

	var cli CoreGlobals
	parser, err := kong.New(&cli, kong.Resolvers(resolver))
	if err != nil {
		t.Fatalf("kong.New failed: %v", err)
	}

	_, err = parser.Parse([]string{})
	if err != nil {
		t.Fatalf("parser.Parse failed: %v", err)
	}

	if cli.ChromaPath != "/tmp/custom-chroma" {
		t.Errorf("Expected ChromaPath '/tmp/custom-chroma', got %q", cli.ChromaPath)
	}
	if cli.VaultPath != "/tmp/my-vault" {
		t.Errorf("Expected VaultPath '/tmp/my-vault', got %q", cli.VaultPath)
	}
	if cli.ContextWindow != 2 {
		t.Errorf("Expected ContextWindow 2, got %d", cli.ContextWindow)
	}
	if cli.MinScore != 0.75 {
		t.Errorf("Expected MinScore 0.75, got %f", cli.MinScore)
	}
	if cli.Debug != true {
		t.Errorf("Expected Debug true, got %v", cli.Debug)
	}
}

func TestTOMLResolver_LogLevel(t *testing.T) {
	tomlData := []byte(`
log_level = "warn"
`)

	resolver, err := TOMLResolver(bytes.NewReader(tomlData))
	if err != nil {
		t.Fatalf("TOMLResolver failed: %v", err)
	}

	var cli CoreGlobals
	parser, err := kong.New(&cli, kong.Resolvers(resolver))
	if err != nil {
		t.Fatalf("kong.New failed: %v", err)
	}

	_, err = parser.Parse([]string{})
	if err != nil {
		t.Fatalf("parser.Parse failed: %v", err)
	}

	if cli.LogLevel != "warn" {
		t.Errorf("Expected LogLevel 'warn', got %q", cli.LogLevel)
	}
}

func TestTOMLResolver_NormalizeCollisionIsDeterministic(t *testing.T) {
	tomlData := []byte(`
show_tags = true
show-tags = false
`)

	// buildNormalizedKeys must resolve the collision identically on every run:
	// "show-tags" sorts before "show_tags", so the first wins.
	want := map[string]any{"showtags": false}
	for i := range 20 {
		parsed := make(map[string]any)
		if err := toml.NewDecoder(bytes.NewReader(tomlData)).Decode(&parsed); err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		got := buildNormalizedKeys(parsed)
		if got["showtags"] != want["showtags"] {
			t.Fatalf("run %d: expected show-tags=%v to win, got %v", i, want["showtags"], got["showtags"])
		}
	}
}

func TestTOMLResolver_SkipFlags(t *testing.T) {
	tomlData := []byte(`
skip_phantom = false
`)

	resolver, err := TOMLResolver(bytes.NewReader(tomlData))
	if err != nil {
		t.Fatalf("TOMLResolver failed: %v", err)
	}

	var cli CoreGlobals
	parser, err := kong.New(&cli, kong.Resolvers(resolver))
	if err != nil {
		t.Fatalf("kong.New failed: %v", err)
	}

	_, err = parser.Parse([]string{})
	if err != nil {
		t.Fatalf("parser.Parse failed: %v", err)
	}

	if cli.SkipPhantom != false {
		t.Errorf("Expected SkipPhantom false, got %v", cli.SkipPhantom)
	}
}

type SearchGlobals struct {
	TopKPerNote int `name:"top-k" default:"3"`
	Limit       int `name:"limit" default:"10"`
}

func TestTOMLResolver_TopK(t *testing.T) {
	tomlData := []byte(`
top-k = 1
limit = 5
`)

	resolver, err := TOMLResolver(bytes.NewReader(tomlData))
	if err != nil {
		t.Fatalf("TOMLResolver failed: %v", err)
	}

	var cli SearchGlobals
	parser, err := kong.New(&cli, kong.Resolvers(resolver))
	if err != nil {
		t.Fatalf("kong.New failed: %v", err)
	}

	_, err = parser.Parse([]string{})
	if err != nil {
		t.Fatalf("parser.Parse failed: %v", err)
	}

	if cli.TopKPerNote != 1 {
		t.Errorf("Expected TopKPerNote 1, got %d", cli.TopKPerNote)
	}
	if cli.Limit != 5 {
		t.Errorf("Expected Limit 5, got %d", cli.Limit)
	}
}

func TestTOMLResolver_DeprecatedHideTags(t *testing.T) {
	tomlData := []byte(`
hide-tags = false
`)

	resolver, err := TOMLResolver(bytes.NewReader(tomlData))
	if err != nil {
		t.Fatalf("TOMLResolver failed: %v", err)
	}

	var cli CoreGlobals
	parser, err := kong.New(&cli, kong.Resolvers(resolver))
	if err != nil {
		t.Fatalf("kong.New failed: %v", err)
	}

	_, err = parser.Parse([]string{})
	if err != nil {
		t.Fatalf("parser.Parse failed: %v", err)
	}

	if cli.ShowTags != true {
		t.Errorf("Expected ShowTags true when hide-tags=false is in TOML, got %v", cli.ShowTags)
	}
}
