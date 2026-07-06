package cmd

import (
	"reflect"
	"testing"
)

func TestResolveQueries(t *testing.T) {
	tests := []struct {
		name    string
		queries []string
		split   bool
		splitBy string
		want    []string
	}{
		{
			name:    "single query no split",
			queries: []string{"redis"},
			split:   false,
			splitBy: ",|;",
			want:    []string{"redis"},
		},
		{
			name:    "multi positional queries no split flag",
			queries: []string{"redis", "message broker"},
			split:   false,
			splitBy: ",|;",
			want:    []string{"redis", "message broker"},
		},
		{
			name:    "single string split by comma and pipe",
			queries: []string{"redis, message broker | backpressure"},
			split:   true,
			splitBy: ",|;",
			want:    []string{"redis", "message broker", "backpressure"},
		},
		{
			name:    "deduplication and whitespace trimming",
			queries: []string{"  redis ", "redis", "message broker; redis"},
			split:   true,
			splitBy: ",|;",
			want:    []string{"redis", "message broker"},
		},
		{
			name:    "empty strings ignored",
			queries: []string{"", "  ", ", , ;"},
			split:   true,
			splitBy: ",|;",
			want:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveQueries(tt.queries, tt.split, tt.splitBy)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("resolveQueries() = %v, want %v", got, tt.want)
			}
		})
	}
}
