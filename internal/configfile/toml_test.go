package configfile

import (
	"bytes"
	"testing"

	"github.com/alecthomas/kong"
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
	LogFormat     string  `name:"log-format" default:"auto"`
	LogLevel      string  `name:"log-level" default:"info"`
}

func TestTOMLResolver_StrictNoHTTP(t *testing.T) {
	tomlData := []byte(`
chroma-path = "/tmp/custom-chroma"
vault_path = "/tmp/my-vault"
context_window = 2
min_score = 0.75
log_format = "json"
log-level = "debug"
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
	if cli.LogFormat != "json" {
		t.Errorf("Expected LogFormat 'json', got %q", cli.LogFormat)
	}
	if cli.LogLevel != "debug" {
		t.Errorf("Expected LogLevel 'debug', got %q", cli.LogLevel)
	}
}
