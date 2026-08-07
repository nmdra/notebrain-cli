package store

import (
	"context"
	"fmt"
	"sort"
	"strings"

	chroma "github.com/amikos-tech/chroma-go/pkg/api/v2"
)

// lexicalTokenMinLen is the minimum token length kept from a query. Single
// characters match too broadly (e.g. "a" or "e") and pollute keyword hits.
const lexicalTokenMinLen = 2

// lexicalTokens splits a query into lowercase keyword tokens, deduplicated.
func lexicalTokens(query string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, raw := range strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	}) {
		if len(raw) < lexicalTokenMinLen {
			continue
		}
		if _, ok := seen[raw]; ok {
			continue
		}
		seen[raw] = struct{}{}
		out = append(out, raw)
	}
	return out
}

// LexicalSearch finds notes whose title, file path, tags, or first-chunk
// text contains the query's tokens (case-insensitive substring). It is the
// fallback for queries that produce no semantic matches (e.g. short common
// words like "Lecture"). Rows are ranked by token hits (title hits weigh
// most) and marked Lexical so callers can distinguish keyword matches from
// semantic similarity; Score is always 0. The whereFilter (tag, section,
// excludes, ...) is respected.
func (s *Store) LexicalSearch(ctx context.Context, query string, limit int, whereFilter WhereFilter) ([]Result, error) {
	tokens := lexicalTokens(query)
	if len(tokens) == 0 {
		return nil, nil
	}

	where, err := CombineWhereFilters(whereFilter, chroma.EqInt("chunk_index", 0))
	if err != nil {
		return nil, fmt.Errorf("lexical search: %w", err)
	}

	s.mu.RLock()
	metas, texts, err := paginatedGetMetadatasWithDocs(ctx, s.chunks, where)
	s.mu.RUnlock()
	if err != nil {
		return nil, fmt.Errorf("lexical search: %w", wrapChromaErr(err))
	}

	type scored struct {
		slug, title, path, text, fileType string
		titleLower, pathLower, textLower  string
		tags                              []string
		score                             int
	}
	bySlug := make(map[string]*scored)
	for i, m := range metas {
		slug := metaString(m, "note_slug")
		if slug == "" {
			continue
		}
		txt := ""
		if len(texts) > i && texts[i] != nil {
			txt = texts[i].ContentString()
		}
		s := &scored{
			slug:       slug,
			title:      metaString(m, "title"),
			path:       metaString(m, "file_path"),
			text:       txt,
			titleLower: strings.ToLower(metaString(m, "title")),
			pathLower:  strings.ToLower(metaString(m, "file_path")),
			textLower:  strings.ToLower(txt),
			tags:       decodeTags(m),
			fileType:   metaString(m, "file_type"),
		}
		bySlug[slug] = s
	}

	for _, s := range bySlug {
		for _, tok := range tokens {
			if strings.Contains(s.titleLower, tok) {
				s.score += 2
			}
			if strings.Contains(s.pathLower, tok) {
				s.score++
			}
			if strings.Contains(s.textLower, tok) {
				s.score++
			}
			for _, t := range s.tags {
				if strings.Contains(strings.ToLower(t), tok) {
					s.score++
				}
			}
		}
	}

	ranked := make([]scored, 0, len(bySlug))
	for _, s := range bySlug {
		if s.score > 0 {
			ranked = append(ranked, *s)
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].slug < ranked[j].slug
	})

	if limit <= 0 {
		limit = len(ranked)
	}
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}

	out := make([]Result, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, Result{
			NoteSlug: r.slug,
			Title:    r.title,
			FilePath: r.path,
			Tags:     r.tags,
			FileType: r.fileType,
			Lexical:  true,
		})
	}
	return out, nil
}
