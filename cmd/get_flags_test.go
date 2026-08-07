package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

func TestGetMetaMode(t *testing.T) {
	fs := &fakeStore{noteMeta: &store.NoteContent{
		NoteSlug: "multi", Title: "Multi Note", FilePath: "Multi.md", Tags: []string{"go"}, Chunks: 3,
	}}
	withFakeStore(t, fs)

	out := captureStdout(t, func() {
		if err := (&GetCmd{Slug: "multi", Meta: true}).Run(&Globals{Ctx: context.Background(), Format: formatText}); err != nil {
			t.Errorf("Run: %v", err)
		}
	})
	if fs.metaCalls != 1 || fs.headCalls != 0 {
		t.Errorf("metaCalls=%d headCalls=%d, want 1/0", fs.metaCalls, fs.headCalls)
	}
	if !strings.Contains(out, "Chunks: 3") || !strings.Contains(out, "Multi Note") {
		t.Errorf("meta text output incomplete:\n%s", out)
	}
	if strings.Contains(out, "chunk zero text") {
		t.Errorf("meta mode must not include note text:\n%s", out)
	}
}

func TestGetHeadMode(t *testing.T) {
	fs := &fakeStore{noteHead: &store.NoteContent{
		NoteSlug: "multi", Title: "Multi Note", Text: "chunk zero text\n\nchunk one text", Chunks: 3,
	}}
	withFakeStore(t, fs)

	out := captureStdout(t, func() {
		if err := (&GetCmd{Slug: "multi", Head: 2}).Run(&Globals{Ctx: context.Background(), Format: formatText}); err != nil {
			t.Errorf("Run: %v", err)
		}
	})
	if fs.metaCalls != 0 || fs.headCalls != 1 {
		t.Errorf("metaCalls=%d headCalls=%d, want 0/1", fs.metaCalls, fs.headCalls)
	}
	if !strings.Contains(out, "chunk zero text") || !strings.Contains(out, "chunk one text") {
		t.Errorf("head text output missing chunks:\n%s", out)
	}
}

func TestGetFullModeUsesGetNote(t *testing.T) {
	fs := &fakeStore{}
	withFakeStore(t, fs)

	_ = captureStdout(t, func() {
		_ = (&GetCmd{Slug: "multi"}).Run(&Globals{Ctx: context.Background(), Format: formatJSON})
	})
	if fs.metaCalls != 0 || fs.headCalls != 0 {
		t.Errorf("full get must not call meta/head (metaCalls=%d headCalls=%d)", fs.metaCalls, fs.headCalls)
	}
}
