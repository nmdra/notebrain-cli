package store

import (
	"context"
	"fmt"
	"sort"
	"strings"

	chroma "github.com/amikos-tech/chroma-go/pkg/api/v2"
	"github.com/amikos-tech/chroma-go/pkg/embeddings"
	"github.com/nmdra/notebrain-cli/v2/internal/parser"
)

// Result is one row returned by any query.
type Result struct {
	NoteSlug       string   `json:"note_slug"`
	Title          string   `json:"title"`
	FilePath       string   `json:"file_path"`
	Score          float64  `json:"score"`
	IsPhantom      bool     `json:"is_phantom,omitempty"`
	ChunkIndex     int      `json:"chunk_index,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	Extra          string   `json:"extra,omitempty"` // e.g. shared tags, hop count
	HeadingPath    string   `json:"heading_path,omitempty"`
	Text           string   `json:"text,omitempty"`    // populated only when include-text is requested
	Context        []string `json:"context,omitempty"` // adjacent chunks when windowing is enabled
	MatchedQueries []string `json:"matched_queries,omitempty"`
}

// NoteContent represents the complete reconstructed text and metadata of a note.
type NoteContent struct {
	NoteSlug string   `json:"note_slug"`
	Title    string   `json:"title"`
	FilePath string   `json:"file_path"`
	Tags     []string `json:"tags,omitempty"`
	Text     string   `json:"text"`
	Chunks   int      `json:"chunks"`
}

// ─── Semantic Search ─────────────────────────────────────────────

// SemanticSearch finds the most similar chunks to queryVec.
// Returns deduplicated chunks retaining up to topKPerNote chunks per note.
func (s *Store) SemanticSearch(ctx context.Context, queryVec []float32, limit int, topKPerNote int, whereFilter chroma.WhereFilter, includeText bool) ([]Result, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.semanticSearch(ctx, queryVec, limit, topKPerNote, whereFilter, includeText)
}

func (s *Store) semanticSearch(ctx context.Context, queryVec []float32, limit int, topKPerNote int, whereFilter chroma.WhereFilter, includeText bool) ([]Result, error) {
	// Fetch enough results to allow top-K deduplication across chunks
	includes := []chroma.Include{chroma.IncludeMetadatas, chroma.IncludeDistances}
	if includeText {
		includes = append(includes, chroma.IncludeDocuments)
	}

	fetchCount := max(limit*3, limit*topKPerNote)

	opts := []chroma.QueryOption{
		chroma.WithQueryEmbeddings(embeddings.NewEmbeddingFromFloat32(queryVec)),
		chroma.WithNResults(fetchCount),
		chroma.WithInclude(includes...),
	}
	if whereFilter != nil {
		opts = append(opts, chroma.WithWhere(whereFilter))
	}

	res, err := s.chunks.Query(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("semantic search: %w", wrapChromaErr(err))
	}
	return deduplicateByNote(res, limit, topKPerNote), nil
}

// MultiSemanticSearch executes semantic searches across multiple query vectors, merging results
// and boosting chunks that match multiple queries.
func (s *Store) MultiSemanticSearch(ctx context.Context, queryVecs [][]float32, queries []string, limit int, topKPerNote int, whereFilter chroma.WhereFilter, includeText bool) ([]Result, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(queryVecs) == 0 {
		return nil, nil
	}
	if len(queryVecs) == 1 {
		res, err := s.semanticSearch(ctx, queryVecs[0], limit, topKPerNote, whereFilter, includeText)
		if err != nil {
			return nil, err
		}
		if len(queries) > 0 && queries[0] != "" {
			for i := range res {
				res[i].MatchedQueries = []string{queries[0]}
			}
		}
		return res, nil
	}

	fetchLimit := max(limit*2, 20)
	fetchTopK := max(topKPerNote*2, 6)

	type mergedChunk struct {
		res     Result
		queries map[string]float32
	}
	chunkMap := make(map[string]*mergedChunk)
	noteQueryScores := make(map[string]map[string]float32)

	for i, vec := range queryVecs {
		qStr := fmt.Sprintf("query_%d", i+1)
		if i < len(queries) && queries[i] != "" {
			qStr = queries[i]
		}
		subRes, err := s.semanticSearch(ctx, vec, fetchLimit, fetchTopK, whereFilter, false)
		if err != nil {
			return nil, fmt.Errorf("semantic search for query %q: %w", qStr, wrapChromaErr(err))
		}
		for _, r := range subRes {
			if r.Score <= 0.0 {
				continue
			}
			qs, ok := noteQueryScores[r.NoteSlug]
			if !ok {
				qs = make(map[string]float32)
				noteQueryScores[r.NoteSlug] = qs
			}
			if float32(r.Score) > qs[qStr] {
				qs[qStr] = float32(r.Score)
			}

			key := fmt.Sprintf("%s:%d", r.NoteSlug, r.ChunkIndex)
			existing, ok := chunkMap[key]
			if !ok {
				chunkMap[key] = &mergedChunk{
					res:     r,
					queries: map[string]float32{qStr: float32(r.Score)},
				}
			} else {
				if oldScore, seen := existing.queries[qStr]; !seen || float32(r.Score) > oldScore {
					existing.queries[qStr] = float32(r.Score)
				}
				if r.Score > existing.res.Score {
					existing.res.Score = r.Score
				}
			}
		}
	}

	merged := make([]Result, 0, len(chunkMap))
	for _, mc := range chunkMap {
		merged = append(merged, mc.res)
	}

	sort.Slice(merged, func(i, j int) bool {
		qi := len(noteQueryScores[merged[i].NoteSlug])
		qj := len(noteQueryScores[merged[j].NoteSlug])
		if qi != qj {
			return qi > qj
		}
		return merged[i].Score > merged[j].Score
	})

	out := deduplicateResultsByNote(merged, limit, topKPerNote)

	type queryScore struct {
		query string
		score float32
	}
	for i := range out {
		qsMap := noteQueryScores[out[i].NoteSlug]
		if len(qsMap) > 0 {
			qsList := make([]queryScore, 0, len(qsMap))
			for q, score := range qsMap {
				qsList = append(qsList, queryScore{query: q, score: score})
			}
			sort.Slice(qsList, func(a, b int) bool {
				return qsList[a].score > qsList[b].score
			})
			sortedQueries := make([]string, len(qsList))
			for j, item := range qsList {
				sortedQueries[j] = item.query
			}
			out[i].MatchedQueries = sortedQueries
		}
	}

	if includeText && len(out) > 0 {
		s.populateTextLocked(ctx, out)
	}
	return out, nil
}

func (s *Store) populateTextLocked(ctx context.Context, results []Result) {
	ids := make([]chroma.DocumentID, len(results))
	idToIndex := make(map[string]int, len(results))
	for i := range results {
		idStr := fmt.Sprintf("%s:%d", results[i].NoteSlug, results[i].ChunkIndex)
		ids[i] = chroma.DocumentID(idStr)
		idToIndex[idStr] = i
	}
	res, err := s.chunks.Get(ctx,
		chroma.WithIDs(ids...),
		chroma.WithInclude(chroma.IncludeDocuments),
	)
	if err != nil || len(res.GetDocuments()) == 0 {
		return
	}
	resIDs := res.GetIDs()
	resDocs := res.GetDocuments()
	for j, id := range resIDs {
		if j < len(resDocs) && resDocs[j] != nil {
			if idx, ok := idToIndex[string(id)]; ok {
				results[idx].Text = resDocs[j].ContentString()
			}
		}
	}
}

func deduplicateResultsByNote(results []Result, limit int, topKPerNote int) []Result {
	if topKPerNote <= 0 {
		topKPerNote = 3
	}
	noteCounts := make(map[string]int)
	var out []Result

	for _, r := range results {
		if noteCounts[r.NoteSlug] >= topKPerNote {
			continue
		}
		noteCounts[r.NoteSlug]++
		out = append(out, r)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

// ─── Metadata Queries ────────────────────────────────────────────

// GetNoteHashes fetches the content_hash for all notes by reading chunk_index=0.
// Returns a map of note_slug -> content_hash.
func (s *Store) GetNoteHashes(ctx context.Context) (map[string]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res, err := s.chunks.Get(ctx,
		chroma.WithWhere(chroma.EqInt("chunk_index", 0)),
		chroma.WithInclude(chroma.IncludeMetadatas),
	)
	if err != nil {
		return nil, fmt.Errorf("get note hashes: %w", wrapChromaErr(err))
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
	s.mu.RLock()
	defer s.mu.RUnlock()
	res, err := s.links.Get(ctx,
		chroma.WithWhere(chroma.EqString("target_slug", targetSlug)),
		chroma.WithInclude(chroma.IncludeMetadatas),
	)
	if err != nil {
		return nil, fmt.Errorf("backlinks: %w", wrapChromaErr(err))
	}

	seen := map[string]bool{}
	var out []Result
	for _, meta := range res.GetMetadatas() {
		if s.SkipAttachments && (parser.IsAttachmentLink(metaString(meta, "target_path")) || parser.IsAttachmentLink(metaString(meta, "display_text"))) {
			continue
		}
		slug := metaString(meta, "source_slug")
		if seen[slug] {
			continue
		}
		seen[slug] = true
		title, filePath, found := s.noteInfoForSlug(ctx, slug)
		out = append(out, Result{
			NoteSlug:  slug,
			Title:     title,
			FilePath:  filePath,
			Score:     1.0,
			Extra:     metaString(meta, "display_text"),
			IsPhantom: !found,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out, nil
}

// ─── Graph Connections (BFS) ──────────────────────────────────────

// Connections finds notes reachable from seedSlug within maxHops.
// BFS implemented in Go (no recursive SQL needed).
func (s *Store) Connections(ctx context.Context, seedSlug string, maxHops int) ([]Result, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
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
					if s.SkipAttachments && (parser.IsAttachmentLink(metaString(meta, "target_path")) || parser.IsAttachmentLink(metaString(meta, "display_text"))) {
						continue
					}
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
					if s.SkipAttachments && (parser.IsAttachmentLink(metaString(meta, "target_path")) || parser.IsAttachmentLink(metaString(meta, "display_text"))) {
						continue
					}
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
		title, filePath, found := s.noteInfoForSlug(ctx, slug)
		out = append(out, Result{
			NoteSlug:  slug,
			Title:     title,
			FilePath:  filePath,
			Score:     float64(hop),
			Extra:     fmt.Sprintf("%d hop(s)", hop),
			IsPhantom: !found,
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
	s.mu.RLock()
	defer s.mu.RUnlock()
	// 1. Collect all slugs already linked to/from seed
	linked := s.linkedSlugs(ctx, seedSlug)
	linked[seedSlug] = true

	// 2. Wide semantic search
	candidates, err := s.semanticSearch(ctx, queryVec, limit*5, 1, nil, includeText)
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

// HiddenConnectionsDeep runs chunk-by-chunk semantic comparison between all chunks of seedSlug
// and all other chunks in the vault. Returns deduplicated results and the labels of the seed chunks analyzed.
func (s *Store) HiddenConnectionsDeep(ctx context.Context, seedSlug string, limit int, topKPerNote int, includeText bool) ([]Result, []string, error) {
	resolved, err := s.ResolveNoteSlug(ctx, seedSlug)
	if err != nil {
		return nil, nil, err
	}
	seedSlug = resolved

	s.mu.RLock()
	// 1. Collect all slugs already linked to/from seed
	linked := s.linkedSlugs(ctx, seedSlug)
	linked[seedSlug] = true

	// 2. Fetch all stored chunks and embeddings for the seed note
	res, err := s.chunks.Get(ctx,
		chroma.WithWhere(chroma.EqString("note_slug", seedSlug)),
		chroma.WithInclude(chroma.IncludeMetadatas, chroma.IncludeEmbeddings),
	)
	s.mu.RUnlock()

	if err != nil {
		return nil, nil, fmt.Errorf("hidden connections deep: get seed chunks: %w", wrapChromaErr(err))
	}

	embs := res.GetEmbeddings()
	metas := res.GetMetadatas()
	if len(embs) == 0 || len(metas) == 0 {
		return nil, nil, fmt.Errorf("note %q has no indexed chunks (required for --deep chunk analysis); run 'notebrain ingest' first", seedSlug)
	}

	type seedChunkInfo struct {
		index int
		vec   []float32
		label string
	}
	var seedInfo []seedChunkInfo
	for i, m := range metas {
		if i >= len(embs) || embs[i] == nil || !embs[i].IsDefined() {
			continue
		}
		vec := embs[i].ContentAsFloat32()
		idx := metaInt(m, "chunk_index")
		hp := metaString(m, "heading_path")
		label := ""
		if hp != "" {
			label = "§ " + hp
		} else if idx == 0 {
			title := metaString(m, "title")
			if title != "" {
				label = "§ " + title
			} else {
				label = "§ (intro)"
			}
		} else {
			label = fmt.Sprintf("chunk #%d", idx+1)
		}
		seedInfo = append(seedInfo, seedChunkInfo{index: idx, vec: vec, label: label})
	}

	if len(seedInfo) == 0 {
		return nil, nil, fmt.Errorf("note %q has no defined chunk vectors", seedSlug)
	}

	sort.Slice(seedInfo, func(i, j int) bool { return seedInfo[i].index < seedInfo[j].index })

	queryVecs := make([][]float32, len(seedInfo))
	seedLabels := make([]string, len(seedInfo))
	for i, info := range seedInfo {
		queryVecs[i] = info.vec
		seedLabels[i] = info.label
	}

	// 3. Wide multi-query semantic search across all vault chunks
	candidates, err := s.MultiSemanticSearch(ctx, queryVecs, seedLabels, max(limit*2, 15), topKPerNote, nil, includeText)
	if err != nil {
		return nil, nil, fmt.Errorf("hidden connections deep: %w", err)
	}

	// 4. Filter out already-linked notes
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
	return out, seedLabels, nil
}

// ─── Shared Tags ──────────────────────────────────────────────────

// SharedTags finds notes sharing at least minShared tags with noteSlug.
func (s *Store) SharedTags(ctx context.Context, noteSlug string, minShared int) ([]Result, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
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
		title, filePath, found := s.noteInfoForSlug(ctx, slug)
		out = append(out, Result{
			NoteSlug:  slug,
			Title:     title,
			FilePath:  filePath,
			Score:     float64(count),
			Tags:      noteTagNames[slug],
			IsPhantom: !found,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out, nil
}

// ─── Direct Tag Search ────────────────────────────────────────────

// CombineWhereFilters safely combines two optional WhereFilters using And.
func CombineWhereFilters(f1, f2 chroma.WhereFilter) chroma.WhereFilter {
	if f1 == nil {
		return f2
	}
	if f2 == nil {
		return f1
	}
	wc1, ok1 := f1.(chroma.WhereClause)
	wc2, ok2 := f2.(chroma.WhereClause)
	if ok1 && ok2 {
		return chroma.And(wc1, wc2)
	}
	return f1
}

// TagWhereClause constructs a Chroma Or filter matching tag against tag_0 through tag_19.
func TagWhereClause(tag string) chroma.WhereClause {
	var clauses []chroma.WhereClause
	for n := range 20 {
		clauses = append(clauses, chroma.EqString(fmt.Sprintf("tag_%d", n), tag))
	}
	return chroma.Or(clauses...)
}

// TagSearch finds notes that match a specific tag name.
func (s *Store) TagSearch(ctx context.Context, tag string, limit int, whereFilter chroma.WhereFilter, includeText bool) ([]Result, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	filter := CombineWhereFilters(TagWhereClause(tag), whereFilter)

	includes := []chroma.Include{chroma.IncludeMetadatas}
	if includeText {
		includes = append(includes, chroma.IncludeDocuments)
	}

	res, err := s.chunks.Get(ctx,
		chroma.WithWhere(filter),
		chroma.WithInclude(includes...),
	)
	if err != nil {
		return nil, fmt.Errorf("tag search: %w", wrapChromaErr(err))
	}

	return getResultToResults(res, limit), nil
}

// ─── Graph-Boosted Search ─────────────────────────────────────────

// GraphBoostedSearch runs semantic search, then boosts scores of notes
// directly linked to/from seedSlug.
func (s *Store) GraphBoostedSearch(ctx context.Context, queryVec []float32, seedSlug string, boost float64, limit int, includeText bool) ([]Result, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	linked := s.linkedSlugs(ctx, seedSlug)

	candidates, err := s.semanticSearch(ctx, queryVec, limit*3, 1, nil, includeText)
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

// deduplicateByNote retains up to topKPerNote chunks per note,
// sorted overall by highest similarity score.
func deduplicateByNote(res chroma.QueryResult, limit int, topKPerNote int) []Result {
	if topKPerNote <= 0 {
		topKPerNote = 3
	}
	groups := res.GetMetadatasGroups()
	if len(groups) == 0 || len(groups[0]) == 0 {
		return nil
	}

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

	noteCounts := make(map[string]int)
	var out []Result

	for i, meta := range metas {
		slug := metaString(meta, "note_slug")
		if noteCounts[slug] >= topKPerNote {
			continue
		}
		noteCounts[slug]++

		dist := float32(0)
		if len(dists) > i {
			dist = float32(dists[i])
		}
		txt := ""
		if len(texts) > i && texts[i] != nil {
			txt = texts[i].ContentString()
		}

		out = append(out, Result{
			NoteSlug:    slug,
			Title:       metaString(meta, "title"),
			FilePath:    metaString(meta, "file_path"),
			Score:       float64(1 - dist), // convert distance → similarity
			HeadingPath: metaString(meta, "heading_path"),
			Text:        txt,
			Tags:        decodeTags(meta),
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func getResultToResults(res chroma.GetResult, limit int) []Result {
	metas := res.GetMetadatas()
	if len(metas) == 0 {
		return nil
	}
	texts := res.GetDocuments()

	type best struct {
		title       string
		filePath    string
		headingPath string
		text        string
		tags        []string
	}
	seen := map[string]*best{}

	for i, meta := range metas {
		slug := metaString(meta, "note_slug")
		if slug == "" {
			continue
		}
		if _, ok := seen[slug]; !ok {
			txt := ""
			if len(texts) > i && texts[i] != nil {
				txt = texts[i].ContentString()
			}
			seen[slug] = &best{
				title:       metaString(meta, "title"),
				filePath:    metaString(meta, "file_path"),
				headingPath: metaString(meta, "heading_path"),
				text:        txt,
				tags:        decodeTags(meta),
			}
		}
	}

	var out []Result
	for slug, b := range seen {
		out = append(out, Result{
			NoteSlug:    slug,
			Title:       b.title,
			FilePath:    b.filePath,
			Score:       1.0,
			HeadingPath: b.headingPath,
			Text:        b.text,
			Tags:        b.tags,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
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
	for n := range 20 {
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
func (s *Store) noteInfoForSlug(ctx context.Context, slug string) (title, filePath string, found bool) {
	res, err := s.chunks.Get(ctx,
		chroma.WithWhere(chroma.And(
			chroma.EqString("note_slug", slug),
			chroma.EqInt("chunk_index", 0),
		)),
		chroma.WithInclude(chroma.IncludeMetadatas),
	)
	if err != nil || len(res.GetMetadatas()) == 0 {
		return slug, "", false
	}
	m := res.GetMetadatas()[0]
	title = metaString(m, "title")
	if title == "" {
		title = slug
	}
	filePath = metaString(m, "file_path")
	return title, filePath, true
}

// metaString safely reads a string from a DocumentMetadata.
func metaString(m chroma.DocumentMetadata, key string) string {
	if s, ok := m.GetString(key); ok {
		return s
	}
	return ""
}

// metaInt safely reads an int from a DocumentMetadata.
func metaInt(m chroma.DocumentMetadata, key string) int {
	if c, ok := m.GetInt(key); ok {
		return int(c)
	} else if c, ok := m.GetFloat(key); ok {
		return int(c)
	}
	return 0
}

// decodeTags reads tag_0, tag_1, … from metadata back into a []string.
func decodeTags(m chroma.DocumentMetadata) []string {
	count := metaInt(m, "tag_count")
	tags := make([]string, 0, count)
	for i := range count {
		key := fmt.Sprintf("tag_%d", i)
		tags = append(tags, metaString(m, key))
	}
	return tags
}

// ResolveNoteSlug resolves a user-provided input (exact slug, title, filename, or partial path)
// to its exact indexed note_slug in ChromaDB.
func (s *Store) ResolveNoteSlug(ctx context.Context, input string) (string, error) {
	cleanInput := strings.TrimSpace(input)
	if cleanInput == "" {
		return "", nil
	}

	slug := parser.Slugify(cleanInput)
	s.mu.RLock()
	res, err := s.chunks.Get(ctx,
		chroma.WithWhere(chroma.Or(
			chroma.EqString("note_slug", slug),
			chroma.EqString("note_slug", cleanInput),
			chroma.EqString("file_path", cleanInput),
		)),
		chroma.WithLimit(1),
	)
	s.mu.RUnlock()
	if err == nil && len(res.GetIDs()) > 0 && len(res.GetMetadatas()) > 0 {
		mSlug := metaString(res.GetMetadatas()[0], "note_slug")
		if mSlug != "" {
			return mSlug, nil
		}
	}

	s.mu.RLock()
	res, err = s.chunks.Get(ctx,
		chroma.WithWhere(chroma.EqInt("chunk_index", 0)),
		chroma.WithInclude(chroma.IncludeMetadatas),
	)
	s.mu.RUnlock()
	if err != nil {
		return slug, nil
	}

	var titleMatches []string
	var pathMatches []string
	var suffixMatches []string

	for _, m := range res.GetMetadatas() {
		mSlug := metaString(m, "note_slug")
		mTitle := metaString(m, "title")
		mPath := metaString(m, "file_path")
		if mSlug == "" {
			continue
		}

		if strings.EqualFold(mTitle, cleanInput) {
			titleMatches = append(titleMatches, mSlug)
			continue
		}

		if strings.EqualFold(mPath, cleanInput) || strings.EqualFold(mPath, cleanInput+".md") {
			pathMatches = append(pathMatches, mSlug)
			continue
		}
		if strings.HasSuffix(strings.ToLower(mPath), "/"+strings.ToLower(cleanInput)) ||
			strings.HasSuffix(strings.ToLower(mPath), "/"+strings.ToLower(cleanInput+".md")) {
			pathMatches = append(pathMatches, mSlug)
			continue
		}

		if slug != "" && strings.HasSuffix(mSlug, slug) {
			suffixMatches = append(suffixMatches, mSlug)
		}
	}

	if len(pathMatches) == 1 {
		return pathMatches[0], nil
	}
	if len(titleMatches) == 1 {
		return titleMatches[0], nil
	}
	if len(suffixMatches) == 1 {
		return suffixMatches[0], nil
	}

	allMatches := make(map[string]struct{})
	for _, s := range pathMatches {
		allMatches[s] = struct{}{}
	}
	for _, s := range titleMatches {
		allMatches[s] = struct{}{}
	}
	for _, s := range suffixMatches {
		allMatches[s] = struct{}{}
	}
	if len(allMatches) == 1 {
		for s := range allMatches {
			return s, nil
		}
	} else if len(allMatches) > 1 {
		matchesList := make([]string, 0, len(allMatches))
		for s := range allMatches {
			matchesList = append(matchesList, s)
		}
		sort.Strings(matchesList)
		return "", fmt.Errorf("note %q matches multiple indexed notes: %s (please specify the exact note slug or path)", input, strings.Join(matchesList, ", "))
	}

	return slug, nil
}

// GetNote retrieves all chunks of a note and reconstructs its complete content.
func (s *Store) GetNote(ctx context.Context, slugOrPath string) (*NoteContent, error) {
	resolved, err := s.ResolveNoteSlug(ctx, slugOrPath)
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	res, err := s.chunks.Get(ctx,
		chroma.WithWhere(chroma.Or(
			chroma.EqString("note_slug", resolved),
			chroma.EqString("file_path", slugOrPath),
		)),
		chroma.WithInclude(chroma.IncludeMetadatas, chroma.IncludeDocuments),
	)
	if err != nil {
		return nil, fmt.Errorf("get note: %w", wrapChromaErr(err))
	}

	metas := res.GetMetadatas()
	texts := res.GetDocuments()
	if len(metas) == 0 {
		return nil, fmt.Errorf("note not found: %s", slugOrPath)
	}

	type chunkInfo struct {
		index int
		text  string
		meta  chroma.DocumentMetadata
	}
	var chunks []chunkInfo
	for i, m := range metas {
		idx := metaInt(m, "chunk_index")
		txt := ""
		if len(texts) > i && texts[i] != nil {
			txt = texts[i].ContentString()
		}
		chunks = append(chunks, chunkInfo{index: idx, text: txt, meta: m})
	}

	sort.Slice(chunks, func(i, j int) bool { return chunks[i].index < chunks[j].index })

	firstMeta := chunks[0].meta
	slug := metaString(firstMeta, "note_slug")
	title := metaString(firstMeta, "title")
	filePath := metaString(firstMeta, "file_path")
	tags := decodeTags(firstMeta)

	var textParts []string
	for _, c := range chunks {
		if c.text != "" {
			textParts = append(textParts, c.text)
		}
	}
	fullText := strings.Join(textParts, "\n\n")

	return &NoteContent{
		NoteSlug: slug,
		Title:    title,
		FilePath: filePath,
		Tags:     tags,
		Text:     fullText,
		Chunks:   len(chunks),
	}, nil
}

// ChunkWindow contains a matched chunk with its surrounding context.
type ChunkWindow struct {
	MatchedIndex int      `json:"matched_index"`
	Texts        []string `json:"texts"`
	Indices      []int    `json:"indices"`
}

// GetChunkWindow fetches ±windowSize adjacent chunks around the given chunk index.
func (s *Store) GetChunkWindow(ctx context.Context, noteSlug string, chunkIndex int, windowSize int) (*ChunkWindow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getChunkWindow(ctx, noteSlug, chunkIndex, windowSize)
}

func (s *Store) getChunkWindow(ctx context.Context, noteSlug string, chunkIndex int, windowSize int) (*ChunkWindow, error) {
	if windowSize <= 0 {
		return nil, nil
	}

	res, err := s.chunks.Get(ctx,
		chroma.WithWhere(chroma.EqString("note_slug", noteSlug)),
		chroma.WithInclude(chroma.IncludeMetadatas, chroma.IncludeDocuments),
	)
	if err != nil {
		return nil, fmt.Errorf("get chunk window: %w", wrapChromaErr(err))
	}

	metas := res.GetMetadatas()
	texts := res.GetDocuments()
	if len(metas) == 0 {
		return nil, nil
	}

	type indexedChunk struct {
		index int
		text  string
	}
	var allChunks []indexedChunk
	for i, m := range metas {
		idx := metaInt(m, "chunk_index")
		txt := ""
		if len(texts) > i && texts[i] != nil {
			txt = texts[i].ContentString()
		}
		allChunks = append(allChunks, indexedChunk{index: idx, text: txt})
	}

	sort.Slice(allChunks, func(i, j int) bool { return allChunks[i].index < allChunks[j].index })

	minIdx := chunkIndex - windowSize
	maxIdx := chunkIndex + windowSize

	window := &ChunkWindow{MatchedIndex: chunkIndex}
	for _, c := range allChunks {
		if c.index >= minIdx && c.index <= maxIdx {
			window.Texts = append(window.Texts, c.text)
			window.Indices = append(window.Indices, c.index)
		}
	}

	return window, nil
}

// PopulateContext fetches adjacent chunks for each result when windowSize > 0.
func (s *Store) PopulateContext(ctx context.Context, results []Result, windowSize int) {
	if windowSize <= 0 {
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range results {
		window, err := s.getChunkWindow(ctx, results[i].NoteSlug, results[i].ChunkIndex, windowSize)
		if err == nil && window != nil {
			results[i].Context = window.Texts
		}
	}
}

// wrapChromaErr annotates ChromaDB decoding errors caused by the embedded FFI 1 MiB string limit.
func wrapChromaErr(err error) error {
	if err == nil {
		return nil
	}
	errStr := err.Error()
	if strings.Contains(errStr, "upstream ChromaDB embedded 1 MiB FFI limit") {
		return err
	}
	if strings.Contains(errStr, "unexpected end of JSON input") || strings.Contains(errStr, "failed to decode") {
		return fmt.Errorf("%w: upstream ChromaDB embedded 1 MiB FFI limit exceeded on large records payload; try lowering --top-k or --limit", err)
	}
	return err
}
