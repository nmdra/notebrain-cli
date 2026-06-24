package store

import (
	"context"
	"fmt"
	"sort"
	"strings"

	chroma "github.com/amikos-tech/chroma-go/pkg/api/v2"
	"github.com/amikos-tech/chroma-go/pkg/embeddings"
)

// Result is one row returned by any query.
type Result struct {
	NoteSlug    string
	Title       string
	FilePath    string
	Score       float64
	Extra       string // e.g. shared tags, hop count
	HeadingPath string
	Text        string // populated only when include-text is requested
}

// ─── Semantic Search ─────────────────────────────────────────────

// SemanticSearch finds the most similar chunks to queryVec.
// Returns deduplicated notes (best chunk per note).
func (s *Store) SemanticSearch(ctx context.Context, queryVec []float32, limit int, whereFilter chroma.WhereFilter, includeText bool) ([]Result, error) {
	// Fetch 3× limit to allow deduplication across chunks of same note
	includes := []chroma.Include{chroma.IncludeMetadatas, chroma.IncludeDistances}
	if includeText {
		includes = append(includes, chroma.IncludeDocuments)
	}

	opts := []chroma.QueryOption{
		chroma.WithQueryEmbeddings(embeddings.NewEmbeddingFromFloat32(queryVec)),
		chroma.WithNResults(limit * 3),
		chroma.WithInclude(includes...),
	}
	if whereFilter != nil {
		opts = append(opts, chroma.WithWhere(whereFilter))
	}

	res, err := s.chunks.Query(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("semantic search: %w", err)
	}
	return deduplicateByNote(res, limit), nil
}

// ─── Metadata Queries ────────────────────────────────────────────

// GetNoteHashes fetches the content_hash for all notes by reading chunk_index=0.
// Returns a map of note_slug -> content_hash.
func (s *Store) GetNoteHashes(ctx context.Context) (map[string]string, error) {
	res, err := s.chunks.Get(ctx,
		chroma.WithWhere(chroma.EqInt("chunk_index", 0)),
		chroma.WithInclude(chroma.IncludeMetadatas),
	)
	if err != nil {
		return nil, fmt.Errorf("get note hashes: %w", err)
	}

	hashes := make(map[string]string)
	for _, m := range res.GetMetadatas() {
		slug := metaString(m, "note_slug")
		hash := metaString(m, "content_hash")
		if slug != "" && hash != "" {
			hashes[slug] = hash
		}
	}
	return hashes, nil
}

// ─── Backlinks ───────────────────────────────────────────────────

// Backlinks returns all notes that link TO targetSlug.
func (s *Store) Backlinks(ctx context.Context, targetSlug string) ([]Result, error) {
	res, err := s.links.Get(ctx,
		chroma.WithWhere(chroma.EqString("target_slug", targetSlug)),
		chroma.WithInclude(chroma.IncludeMetadatas),
	)
	if err != nil {
		return nil, fmt.Errorf("backlinks: %w", err)
	}

	seen := map[string]bool{}
	var out []Result
	for _, meta := range res.GetMetadatas() {
		slug := metaString(meta, "source_slug")
		if seen[slug] {
			continue
		}
		seen[slug] = true
		title, filePath := s.noteInfoForSlug(ctx, slug)
		out = append(out, Result{
			NoteSlug: slug,
			Title:    title,
			FilePath: filePath,
			Score:    1.0,
			Extra:    metaString(meta, "display_text"),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out, nil
}

// ─── Graph Connections (BFS) ──────────────────────────────────────

// Connections finds notes reachable from seedSlug within maxHops.
// BFS implemented in Go (no recursive SQL needed).
func (s *Store) Connections(ctx context.Context, seedSlug string, maxHops int) ([]Result, error) {
	visited := map[string]int{seedSlug: 0} // slug → hop count
	frontier := []string{seedSlug}

	for hop := 1; hop <= maxHops && len(frontier) > 0; hop++ {
		var next []string
		for _, src := range frontier {
			// Outgoing links
			out, _ := s.links.Get(ctx,
				chroma.WithWhere(chroma.EqString("source_slug", src)),
				chroma.WithInclude(chroma.IncludeMetadatas),
			)
			if out != nil {
				for _, meta := range out.GetMetadatas() {
					tgt := metaString(meta, "target_slug")
					if _, ok := visited[tgt]; !ok {
						visited[tgt] = hop
						next = append(next, tgt)
					}
				}
			}
			// Incoming links (bidirectional)
			in, _ := s.links.Get(ctx,
				chroma.WithWhere(chroma.EqString("target_slug", src)),
				chroma.WithInclude(chroma.IncludeMetadatas),
			)
			if in != nil {
				for _, meta := range in.GetMetadatas() {
					tgt := metaString(meta, "source_slug")
					if _, ok := visited[tgt]; !ok {
						visited[tgt] = hop
						next = append(next, tgt)
					}
				}
			}
		}
		frontier = next
	}

	delete(visited, seedSlug)
	var out []Result
	for slug, hop := range visited {
		title, filePath := s.noteInfoForSlug(ctx, slug)
		out = append(out, Result{
			NoteSlug: slug,
			Title:    title,
			FilePath: filePath,
			Score:    float64(hop),
			Extra:    fmt.Sprintf("%d hop(s)", hop),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score < out[j].Score
		}
		return out[i].Title < out[j].Title
	})
	return out, nil
}

// ─── Hidden Connections ───────────────────────────────────────────

// HiddenConnections finds notes semantically similar to queryVec
// but NOT already linked to/from seedSlug.
func (s *Store) HiddenConnections(ctx context.Context, queryVec []float32, seedSlug string, limit int, includeText bool) ([]Result, error) {
	// 1. Collect all slugs already linked to/from seed
	linked := s.linkedSlugs(ctx, seedSlug)
	linked[seedSlug] = true

	// 2. Wide semantic search
	candidates, err := s.SemanticSearch(ctx, queryVec, limit*5, nil, includeText)
	if err != nil {
		return nil, err
	}

	// 3. Filter out already-linked notes
	var out []Result
	for _, r := range candidates {
		if linked[r.NoteSlug] {
			continue
		}
		out = append(out, r)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// ─── Shared Tags ──────────────────────────────────────────────────

// SharedTags finds notes sharing at least minShared tags with noteSlug.
func (s *Store) SharedTags(ctx context.Context, noteSlug string, minShared int) ([]Result, error) {
	// 1. Get tags of the seed note (from its first chunk)
	seedTags := s.tagsForNote(ctx, noteSlug)
	if len(seedTags) == 0 {
		return nil, nil
	}

	// 2. For each seed tag, find all notes that have it
	noteTagCount := map[string]int{}      // slug → shared tag count
	noteTagNames := map[string][]string{} // slug → which tags are shared

	for _, tag := range seedTags {
		slugs := s.notesWithTag(ctx, tag)
		for _, slug := range slugs {
			if slug == noteSlug {
				continue
			}
			noteTagCount[slug]++
			noteTagNames[slug] = append(noteTagNames[slug], tag)
		}
	}

	// 3. Filter by minShared
	var out []Result
	for slug, count := range noteTagCount {
		if count < minShared {
			continue
		}
		title, filePath := s.noteInfoForSlug(ctx, slug)
		out = append(out, Result{
			NoteSlug: slug,
			Title:    title,
			FilePath: filePath,
			Score:    float64(count),
			Extra:    strings.Join(noteTagNames[slug], ", "),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out, nil
}

// ─── Graph-Boosted Search ─────────────────────────────────────────

// GraphBoostedSearch runs semantic search, then boosts scores of notes
// directly linked to/from seedSlug.
func (s *Store) GraphBoostedSearch(ctx context.Context, queryVec []float32, seedSlug string, boost float64, limit int, includeText bool) ([]Result, error) {
	linked := s.linkedSlugs(ctx, seedSlug)

	candidates, err := s.SemanticSearch(ctx, queryVec, limit*3, nil, includeText)
	if err != nil {
		return nil, err
	}

	for i, r := range candidates {
		if linked[r.NoteSlug] {
			candidates[i].Score *= boost
			candidates[i].Extra = "graph-boosted"
		}
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Score > candidates[j].Score })
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates, nil
}

// ─── Internal helpers ─────────────────────────────────────────────

// deduplicateByNote collapses multiple chunks from the same note,
// keeping the best (lowest distance) chunk per note.
func deduplicateByNote(res chroma.QueryResult, limit int) []Result {
	groups := res.GetMetadatasGroups()
	if len(groups) == 0 || len(groups[0]) == 0 {
		return nil
	}

	type best struct {
		title       string
		filePath    string
		distance    float32
		headingPath string
		text        string
	}
	seen := map[string]*best{}
	metas := groups[0]

	var dists []embeddings.Distance
	distGroups := res.GetDistancesGroups()
	if len(distGroups) > 0 {
		dists = distGroups[0]
	}

	var texts chroma.Documents
	docsGroups := res.GetDocumentsGroups()
	if len(docsGroups) > 0 {
		texts = docsGroups[0]
	}

	for i, meta := range metas {
		slug := metaString(meta, "note_slug")
		dist := float32(0)
		if len(dists) > i {
			dist = float32(dists[i])
		}
		if b, ok := seen[slug]; !ok || dist < b.distance {
			txt := ""
			if len(texts) > i && texts[i] != nil {
				txt = texts[i].ContentString()
			}
			seen[slug] = &best{
				title:       metaString(meta, "title"),
				filePath:    metaString(meta, "file_path"),
				distance:    dist,
				headingPath: metaString(meta, "heading_path"),
				text:        txt,
			}
		}
	}
	var out []Result
	for slug, b := range seen {
		out = append(out, Result{
			NoteSlug:    slug,
			Title:       b.title,
			FilePath:    b.filePath,
			Score:       float64(1 - b.distance), // convert distance → similarity
			HeadingPath: b.headingPath,
			Text:        b.text,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// linkedSlugs returns a set of note slugs linked to/from slug.
func (s *Store) linkedSlugs(ctx context.Context, slug string) map[string]bool {
	set := map[string]bool{}
	out, _ := s.links.Get(ctx,
		chroma.WithWhere(chroma.EqString("source_slug", slug)),
		chroma.WithInclude(chroma.IncludeMetadatas),
	)
	if out != nil {
		for _, m := range out.GetMetadatas() {
			if t := metaString(m, "target_slug"); t != "" {
				set[t] = true
			}
		}
	}
	in, _ := s.links.Get(ctx,
		chroma.WithWhere(chroma.EqString("target_slug", slug)),
		chroma.WithInclude(chroma.IncludeMetadatas),
	)
	if in != nil {
		for _, m := range in.GetMetadatas() {
			if src := metaString(m, "source_slug"); src != "" {
				set[src] = true
			}
		}
	}
	return set
}

// tagsForNote fetches the tags of the first chunk of noteSlug.
func (s *Store) tagsForNote(ctx context.Context, noteSlug string) []string {
	res, err := s.chunks.Get(ctx,
		chroma.WithWhere(chroma.And(
			chroma.EqString("note_slug", noteSlug),
			chroma.EqInt("chunk_index", 0),
		)),
		chroma.WithInclude(chroma.IncludeMetadatas),
	)
	if err != nil || len(res.GetMetadatas()) == 0 {
		return nil
	}
	return decodeTags(res.GetMetadatas()[0])
}

// notesWithTag returns distinct note slugs that have the given tag.
func (s *Store) notesWithTag(ctx context.Context, tag string) []string {
	// Query all chunks for each possible tag_N position (up to 20 tags)
	var filters []chroma.WhereClause
	for n := 0; n < 20; n++ {
		filters = append(filters, chroma.EqString(fmt.Sprintf("tag_%d", n), tag))
	}
	res, err := s.chunks.Get(ctx,
		chroma.WithWhere(chroma.Or(filters...)),
		chroma.WithInclude(chroma.IncludeMetadatas),
	)
	if err != nil || len(res.GetMetadatas()) == 0 {
		return nil
	}

	seen := map[string]bool{}
	var slugs []string
	for _, m := range res.GetMetadatas() {
		slug := metaString(m, "note_slug")
		if slug != "" && !seen[slug] {
			seen[slug] = true
			slugs = append(slugs, slug)
		}
	}
	return slugs
}

// noteInfoForSlug fetches the title and file path of a note's first chunk.
func (s *Store) noteInfoForSlug(ctx context.Context, slug string) (title, filePath string) {
	res, err := s.chunks.Get(ctx,
		chroma.WithWhere(chroma.And(
			chroma.EqString("note_slug", slug),
			chroma.EqInt("chunk_index", 0),
		)),
		chroma.WithInclude(chroma.IncludeMetadatas),
	)
	if err != nil || len(res.GetMetadatas()) == 0 {
		return slug, ""
	}
	m := res.GetMetadatas()[0]
	title = metaString(m, "title")
	if title == "" {
		title = slug
	}
	filePath = metaString(m, "file_path")
	return title, filePath
}

// metaString safely reads a string from a DocumentMetadata.
func metaString(m chroma.DocumentMetadata, key string) string {
	if s, ok := m.GetString(key); ok {
		return s
	}
	return ""
}

// decodeTags reads tag_0, tag_1, … from metadata back into a []string.
func decodeTags(m chroma.DocumentMetadata) []string {
	count := 0
	if c, ok := m.GetInt("tag_count"); ok {
		count = int(c)
	} else if c, ok := m.GetFloat("tag_count"); ok {
		count = int(c)
	}
	tags := make([]string, 0, count)
	for i := 0; i < count; i++ {
		key := fmt.Sprintf("tag_%d", i)
		tags = append(tags, metaString(m, key))
	}
	return tags
}
