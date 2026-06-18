package parser

import (
	"reflect"
	"testing"
)

func TestTitleFromPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"Simple", "file.md", "file"},
		{"With Directory", "dir/file.md", "file"},
		{"No Extension", "file", "file"},
		{"Spaces", "dir/my file.md", "my file"},
		{"Dots in name", "my.note.v1.md", "my.note.v1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TitleFromPath(tt.path); got != tt.want {
				t.Errorf("TitleFromPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractLinks(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []Link
	}{
		{
			name: "No links",
			text: "Just some regular text",
			want: nil,
		},
		{
			name: "Simple link",
			text: "Check out [[Note A]] for details",
			want: []Link{{Target: "Note A", DisplayText: "Note A"}},
		},
		{
			name: "Link with display text",
			text: "Check out [[Note A|this note]] for details",
			want: []Link{{Target: "Note A", DisplayText: "this note"}},
		},
		{
			name: "Multiple links",
			text: "[[Note A]] and [[Note B|B]]",
			want: []Link{
				{Target: "Note A", DisplayText: "Note A"},
				{Target: "Note B", DisplayText: "B"},
			},
		},
		{
			name: "Deduplication",
			text: "[[Note A]] and again [[Note A|same target]]",
			want: []Link{{Target: "Note A", DisplayText: "Note A"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractLinks(tt.text)
			if len(got) == 0 && len(tt.want) == 0 {
				return // nil and empty slice are equivalent here
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractLinks() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractTags(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		frontmatter string
		want        []string
	}{
		{
			name: "No tags",
			text: "Just some text without tags",
			want: nil,
		},
		{
			name: "Simple tags",
			text: "This is a #tag and another #test_tag",
			want: []string{"tag", "test_tag"},
		},
		{
			name: "Tags with slash",
			text: "Hierarchical #dev/go",
			want: []string{"dev/go"},
		},
		{
			name: "Deduplication",
			text: "Duplicate #tag and #tag",
			want: []string{"tag"},
		},
		{
			name: "Case insensitivity",
			text: "Upper #TAG and lower #tag",
			want: []string{"tag"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractTags(tt.text, tt.frontmatter)
			if len(got) == 0 && len(tt.want) == 0 {
				return // nil and empty slice are equivalent here
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractTags() = %v, want %v", got, tt.want)
			}
		})
	}
}
