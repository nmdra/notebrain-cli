// Package mcpbridge exposes the retrieval CLI through an isolated MCP adapter.
package mcpbridge

// ChunkOptions apply only to commands that return chunks. Pointers preserve explicit zero and false.
type ChunkOptions struct {
	IncludeText   *bool    `json:"include_text,omitempty"`
	ContextWindow *int     `json:"context_window,omitempty"`
	MinScore      *float64 `json:"min_score,omitempty"`
	GroupByNote   *bool    `json:"group_by_note,omitempty"`
}
type SearchInput struct {
	Queries      *[]string `json:"queries,omitempty"`
	Tag          *string   `json:"tag,omitempty"`
	Section      *string   `json:"section,omitempty"`
	HasTasks     *bool     `json:"has_tasks,omitempty"`
	HasCode      *bool     `json:"has_code,omitempty"`
	WithPDF      *bool     `json:"with_pdf,omitempty"`
	ExcludeNotes *[]string `json:"exclude_notes,omitempty"`
	Limit        *int      `json:"limit,omitempty"`
	TopK         *int      `json:"top_k,omitempty"`
	ChunkOptions
}
type GetInput struct {
	Note string `json:"note"`
	Meta *bool  `json:"meta,omitempty"`
	Head *int   `json:"head,omitempty"`
}
type RefsInput struct {
	Note              string `json:"note"`
	OnlyImages        *bool  `json:"only_images,omitempty"`
	OnlyPDF           *bool  `json:"only_pdf,omitempty"`
	OnlyOther         *bool  `json:"only_other,omitempty"`
	OnlyExternalLinks *bool  `json:"only_external_links,omitempty"`
	IncludeMissing    *bool  `json:"include_missing,omitempty"`
}
type BacklinksInput struct {
	Note string `json:"note"`
}
type ConnectionsInput struct {
	Note string `json:"note"`
	Hops *int   `json:"hops,omitempty"`
}
type HiddenInput struct {
	Note            string `json:"note"`
	Deep            *bool  `json:"deep,omitempty"`
	IncludeLinked   *bool  `json:"include_linked,omitempty"`
	CandidateChunks *int   `json:"candidate_chunks,omitempty"`
	Limit           *int   `json:"limit,omitempty"`
	ChunkOptions
}
type TagsInput struct {
	Query     *string `json:"query,omitempty"`
	List      *bool   `json:"list,omitempty"`
	Shared    *bool   `json:"shared,omitempty"`
	ForNote   *bool   `json:"for_note,omitempty"`
	Children  *bool   `json:"children,omitempty"`
	MinShared *int    `json:"min_shared,omitempty"`
	Limit     *int    `json:"limit,omitempty"`
	ChunkOptions
}
type BoostedInput struct {
	Query   string   `json:"query"`
	Seed    string   `json:"seed"`
	Boost   *float64 `json:"boost,omitempty"`
	WithPDF *bool    `json:"with_pdf,omitempty"`
	Limit   *int     `json:"limit,omitempty"`
	ChunkOptions
}
type StatsInput struct{}
