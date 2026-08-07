package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

func TestTagsListText(t *testing.T) {
	fs := &fakeStore{tags: []store.TagCount{{Tag: "go", Count: 2}, {Tag: "vector", Count: 1}}}
	withFakeStore(t, fs)

	out := captureStdout(t, func() {
		if err := (&TagsCmd{List: true}).Run(&Globals{Ctx: context.Background()}); err != nil {
			t.Errorf("Run: %v", err)
		}
	})
	for _, want := range []string{"#go", "(2 notes)", "#vector", "(1 note)"} {
		if !strings.Contains(out, want) {
			t.Errorf("text output missing %q:\n%s", want, out)
		}
	}
}

func TestTagsListJSON(t *testing.T) {
	fs := &fakeStore{tags: []store.TagCount{{Tag: "go", Count: 2}}}
	withFakeStore(t, fs)

	out := captureStdout(t, func() {
		if err := (&TagsCmd{List: true}).Run(&Globals{Ctx: context.Background(), Format: formatJSON}); err != nil {
			t.Errorf("Run: %v", err)
		}
	})
	for _, want := range []string{`"command"`, `"tags --list"`, `"tag": "go"`, `"count": 2`, `"total": 1`} {
		if !strings.Contains(out, want) {
			t.Errorf("json output missing %s:\n%s", want, out)
		}
	}
}

func TestTagsListTSV(t *testing.T) {
	fs := &fakeStore{tags: []store.TagCount{{Tag: "go", Count: 2}, {Tag: "vector", Count: 1}}}
	withFakeStore(t, fs)

	out := captureStdout(t, func() {
		if err := (&TagsCmd{List: true}).Run(&Globals{Ctx: context.Background(), Format: formatTSV}); err != nil {
			t.Errorf("Run: %v", err)
		}
	})
	want := "tag\tcount\ngo\t2\nvector\t1\n"
	if out != want {
		t.Errorf("tsv output = %q, want %q", out, want)
	}
}

func TestTagsListLimit(t *testing.T) {
	fs := &fakeStore{tags: []store.TagCount{{Tag: "go", Count: 2}, {Tag: "vector", Count: 1}}}
	withFakeStore(t, fs)

	captured := 0
	out := captureStdout(t, func() {
		err := (&TagsCmd{List: true, Limit: 1}).Run(&Globals{Ctx: context.Background()})
		if err == nil {
			captured++
		}
	})
	if captured != 1 {
		t.Fatalf("Run with --limit 1: err=%v", captured)
	}
	if strings.Contains(out, "#vector") {
		t.Errorf("limit 1 output should omit #vector:\n%s", out)
	}
}

func TestTagsDidYouMeanText(t *testing.T) {
	fs := &fakeStore{suggest: []string{"go", "golang"}}
	withFakeStore(t, fs)

	out := captureStdout(t, func() {
		if err := (&TagsCmd{Query: "gol"}).Run(&Globals{Ctx: context.Background(), Format: formatText}); err != nil {
			t.Errorf("Run: %v", err)
		}
	})
	if !strings.Contains(out, "Did you mean") || !strings.Contains(out, "#go") || !strings.Contains(out, "#golang") {
		t.Errorf("text output missing did-you-mean hint:\n%s", out)
	}
}

func TestTagsDidYouMeanNotInJSON(t *testing.T) {
	fs := &fakeStore{suggest: []string{"go"}}
	withFakeStore(t, fs)

	out := captureStdout(t, func() {
		if err := (&TagsCmd{Query: "gol"}).Run(&Globals{Ctx: context.Background(), Format: formatJSON}); err != nil {
			t.Errorf("Run: %v", err)
		}
	})
	if strings.Contains(out, "Did you mean") {
		t.Errorf("json output must not contain did-you-mean hint:\n%s", out)
	}
}

func TestTagsNoQueryRequiresList(t *testing.T) {
	fs := &fakeStore{}
	withFakeStore(t, fs)

	err := (&TagsCmd{}).Run(&Globals{Ctx: context.Background()})
	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected UsageError, got %v", err)
	}
	if !strings.Contains(err.Error(), "--list") {
		t.Errorf("usage error should mention --list, got %q", err.Error())
	}
}

func TestTagsListIgnoresQuery(t *testing.T) {
	fs := &fakeStore{tags: []store.TagCount{{Tag: "go", Count: 1}}}
	withFakeStore(t, fs)

	out := captureStdout(t, func() {
		if err := (&TagsCmd{List: true, Query: "ignore-me"}).Run(&Globals{Ctx: context.Background()}); err != nil {
			t.Errorf("Run: %v", err)
		}
	})
	if !strings.Contains(out, "#go") {
		t.Errorf("--list should ignore the query and list tags:\n%s", out)
	}
}
