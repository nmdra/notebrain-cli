package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// TagCount is a single tag and the number of indexed notes carrying it.
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// ListTags returns every distinct tag in the index with the number of notes
// carrying it, sorted by count descending then tag ascending. A limit <= 0
// returns all tags; otherwise only the top-limit entries are returned.
func (s *Store) ListTags(ctx context.Context, limit int) ([]TagCount, error) {
	s.mu.RLock()
	metas, err := s.paginatedZeroIndexMetadatas(ctx)
	s.mu.RUnlock()
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", wrapChromaErr(err))
	}

	// One zero-index metadata record exists per note; counting notes, not
	// chunks, is done by collecting tags per record.
	counts := make(map[string]int)
	for _, m := range metas {
		for _, t := range decodeTags(m) {
			if t != "" {
				counts[t]++
			}
		}
	}

	tags := make([]TagCount, 0, len(counts))
	for tag, n := range counts {
		tags = append(tags, TagCount{Tag: tag, Count: n})
	}
	sort.Slice(tags, func(i, j int) bool {
		if tags[i].Count != tags[j].Count {
			return tags[i].Count > tags[j].Count
		}
		return tags[i].Tag < tags[j].Tag
	})

	if limit > 0 && len(tags) > limit {
		tags = tags[:limit]
	}
	return tags, nil
}

// SuggestTags returns up to limit tags closest to input by edit distance,
// for "did you mean" hints after a tag search finds nothing. Exact matches
// are excluded. Ties are broken by note count descending, then alphabetically.
func (s *Store) SuggestTags(ctx context.Context, input string, limit int) ([]string, error) {
	all, err := s.ListTags(ctx, 0)
	if err != nil {
		return nil, err
	}
	return closestTags(all, input, limit), nil
}

// closestTags ranks tags by Levenshtein distance to input (ties broken by
// count descending, then tag ascending) and returns the top-limit of them,
// excluding the exact input itself.
func closestTags(all []TagCount, input string, limit int) []string {
	in := strings.ToLower(strings.TrimSpace(input))

	type scored struct {
		dist  int
		count int
		tag   string
	}
	scoredTags := make([]scored, 0, len(all))
	for _, t := range all {
		lower := strings.ToLower(t.Tag)
		if lower == in {
			continue
		}
		scoredTags = append(scoredTags, scored{dist: levenshtein(in, lower), count: t.Count, tag: t.Tag})
	}
	sort.Slice(scoredTags, func(i, j int) bool {
		if scoredTags[i].dist != scoredTags[j].dist {
			return scoredTags[i].dist < scoredTags[j].dist
		}
		if scoredTags[i].count != scoredTags[j].count {
			return scoredTags[i].count > scoredTags[j].count
		}
		return scoredTags[i].tag < scoredTags[j].tag
	})

	if limit <= 0 || len(scoredTags) < limit {
		limit = len(scoredTags)
	}
	out := make([]string, 0, limit)
	for _, s := range scoredTags[:limit] {
		out = append(out, s.tag)
	}
	return out
}

// levenshtein computes the edit distance between two strings (case-sensitive;
// callers normalize case beforehand).
func levenshtein(a, b string) int {
	if a == "" {
		return len(b)
	}
	if b == "" {
		return len(a)
	}
	ra, rb := []rune(a), []rune(b)
	if len(ra) > len(rb) {
		ra, rb = rb, ra
	}
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}
