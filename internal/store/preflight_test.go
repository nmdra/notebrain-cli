package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMCPPreflight(t *testing.T) {
	for _, tc := range []struct {
		name  string
		data  []byte
		valid bool
	}{
		{"missing", nil, false}, {"empty", []byte{}, false}, {"invalid", []byte("not a database file"), false},
		{"valid header", append([]byte("SQLite format 3\x00"), make([]byte, 496)...), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "store")
			if tc.data != nil {
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(path, "chroma.sqlite3"), tc.data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			err := CheckExistingDatabase(context.Background(), path)
			if (err == nil) != tc.valid {
				t.Fatalf("CheckExistingDatabase error=%v, valid=%v", err, tc.valid)
			}
			if tc.data == nil {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("preflight created missing store: %v", err)
				}
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := CheckExistingDatabase(ctx, t.TempDir()); err == nil {
		t.Fatal("canceled preflight succeeded")
	}
}
