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
