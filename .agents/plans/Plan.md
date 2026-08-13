# Plan: `notebrain refs` command

## Goal

Add a new `notebrain refs <note-slug/note-name>` command that lists the **direct references** of a markdown note: local attachment file paths (images, PDFs, archives, …) **and external website links** (URLs), so AI agents can fetch the actual files and sources instead of only note text. The command resolves the note exactly like sibling commands (slug, title, filename, or partial path) and prints absolute file paths / URLs in text/JSON/TSV, filterable with `--images`, `--pdf`, `--other`, `--external-links`.

Outcome: an agent can run `notebrain refs kubernetes-notes --images --format=json` and get `["/vault/assets/arch.png", ...]`, or `notebrain refs kubernetes-notes --external-links --format=json` and get the note's website references.

## Current State

**Note resolution already exists and is the right seam.**

- `store.ResolveNoteSlug` resolves exact slug, title, filename, or partial path in one metadata scan (`internal/store/query.go:1477`).
- `GetNoteMeta` returns `NoteContent{NoteSlug, Title, FilePath, Tags, Chunks}` (`internal/store/query.go:1624`, struct at `query.go:118`) — `FilePath` is vault-relative.
- `storeAPI` in `cmd/helpers.go:16` already exposes `GetNoteMeta`; the fake store used by cmd tests already implements it (`cmd/testhelpers_test.go:102`). **No store changes needed.**

**The index does NOT track attachments — the command must parse the note file fresh from the vault.**

- `metadataTransformer` deliberately skips attachment embeds (`![[img.png]]`) from the link set (`internal/parser/ast.go:105-110`), and `upsertLinks` drops attachment links when `SkipAttachments` is set (`internal/store/upsert.go:200-201`). `nb_links` therefore cannot answer "what does this note reference".

**Classification primitives already exist in `internal/parser/parser.go`.**

- `attachmentExts` allowlist (parser.go:64) — images, video, audio, canvas, archives, office docs. PDFs are deliberately excluded there because they are ingested as notes (parser.go:62).
- `imageExts` (parser.go:98), `IsAttachmentLink` (parser.go:76, strips `|alias` and `#anchor`), private `attachmentKind` (parser.go:108).
- The goldmark AST walk pattern to copy is `metadataTransformer` (`ast.go:89-119`); `mdParser` is a shared package-level goldmark instance with the wikilink extension (`ast.go:52-70`).

**External-link nodes are already reachable in the same AST walk.**

- `mdParser` enables `extension.GFM` (ast.go:60), which includes the Linkify extension (`vendor/github.com/yuin/goldmark/extension/gfm.go:14`) producing `*ast.AutoLink` nodes for bare `http://`/`https://`/`ftp://` and `www.` URLs (`vendor/github.com/yuin/goldmark/extension/linkify.go:14-16`); the node stores `AutoLinkType` (URL vs Email) and `Protocol` separately from the URL text (`vendor/github.com/yuin/goldmark/ast/inline.go:557-567`), which the renderer assembles as `protocol + "://" + text` (`vendor/github.com/yuin/goldmark/renderer/html/html.go:506-515`).
- Markdown links and image embeds are `*ast.Link` / `*ast.Image` nodes with a raw `Destination` (`vendor/github.com/yuin/goldmark/ast/inline.go:447-527`); `handleInlineNode` already renders these kinds (`internal/parser/ast.go:606`).
- Obsidian renders `[[https://…]]` wikilinks as external links, so URL-prefixed wikilink targets belong in the same collection.

**Vault evidence (user's vault, `/home/nimendra/Documents/Second Brain 2.0`, 824 notes)** — the feature is scoped to observed reality:

- 1,683 wiki-link image embeds vs 9 markdown image embeds and 4 markdown links to local files → wiki syntax dominates, but markdown links exist and are in scope (user decision).
- Markdown destinations are percent-encoded in practice (`Router%20Modes.webp`, `1101-Practical-Router-Configuration.md`) → resolution must percent-decode.
- 108 wiki-links to PDFs → `--pdf` filter earns its keep.
- 2,269 lines with http(s) URLs → external-link listing is a real need.
- `attachmentFolderPath = "99.Storage-Shed/Attachments"` in `.obsidian/app.json` → centralized attachment folder confirms the resolution order (note folder → vault root → attachment folder).

**Path resolution rules (Obsidian semantics).**

- Wiki-link targets containing a folder path start at the vault root: `[[Projects/Three laws of motion]]` (<https://obsidian.md/help/links>). Links to non-markdown files require the extension: `[[Figure 1.png]]` (same source).
- Bare wiki names (no `/`) resolve by searching the note's own folder, then the vault root, then the configured attachment folder; we verify each candidate with `os.Stat`, so the search order only decides which existing file wins. `./` prefix resolves relative to the note's folder.
- **Markdown links** (`[doc](file.pdf)`, `![alt](img.png)`) resolve **relative to the note's folder** (Obsidian/CommonMark semantics — unlike wiki links), with percent-decoding; fragment (`#page=3`) stripped.
- The attachment folder is `attachmentFolderPath` in `.obsidian/app.json`, already parsed by `ingest.LoadExcludedPaths` (`internal/ingest/ignore.go:15-36`) — but only the merged ignore-filters list is returned, not the folder itself. A small refactor/exposure is needed.

**Command conventions to follow.**

- Registration: `CLI` struct in `cmd/cli.go:89-122`; flag groups, `completion-predictor:"note-slug"` for the positional arg (as in `TagsCmd`, `cmd/tags.go:23-34`). No name collision: `refs` is distinct from all existing commands (`ingest`, `search`, `backlinks`, `connections`, `hidden`, `tags`, `boosted`, `stats`, `get`, `reset`, `doctor`, `init`, `version`, `completion`).
- Missing vault path → `UsageError` with the exact message pattern from `cmd/ingest.go:47-49`.
- Output follows `printTagsFormattedToWriter` (`cmd/print.go:444-476`): JSON envelope object + `--jsonpath` support via `printJSONPathResultToWriter` (print.go:392), TSV with header row, simple text lines. Formats: `formatText`/`formatJSON`/`formatTSV`.

**Git workflow (branch-per-feature convention).**

- The repo develops features on `feat/*` branches off `master` (evidence: `feat/deep-hidden-connections`, `feat/shell-completion`, `feat/pdf-ocr-support`, `feat/goldmark-extensions`, `feat/cli-ux-improvements` all exist locally/remotely), with Conventional Commits (AGENTS.md) and Lefthook pre-commit hooks.
- This feature gets its own branch: `feat/refs-command`. `master` stays untouched until the user reviews and approves the merge.

**Docs that must stay in sync** (last 4 commits were skill rewrites — the skill is the agent-facing contract):

- `README.md` (Features bullet list at line 26-41; Quick Start chaining example at step 6)
- `wiki/Commands.md` (command sections; insert after `get`, ~line 256)
- `.agents/skills/notebrain/SKILL.md` (retrieval ladder, section "Retrieval ladder" at line 32)
- `.agents/skills/notebrain/references/flags.md` (per-command flag tables)
- `.agents/skills/notebrain/references/example.md` (worked scenarios)
- `.agents/skills/notebrain/references/schema.md` (JSON envelope + example outputs; the `refs` envelope is a new shape)
- `.agents/AGENTS.md` (project structure tree + CLI testing standards)
- `CHANGELOG.md` (v2.12.0 is latest; no `Unreleased` section yet)

**Skill development environment (for the skill update task).**

- The `skill-creator` skill (`/home/nimendra/.agents/skills/skill-creator/`) defines the create → evaluate → iterate loop: snapshot baseline, with-skill vs baseline runs, grading via `agents/grader.md`, `scripts/aggregate_benchmark.py`, and `eval-viewer/generate_review.py`. The `writing-for-agents` skill (`/home/nimendra/.agents/skills/writing-for-agents/`, incl. `SKILL-MECHANICS.md`) defines the craft rules for skill text (progressive disclosure, leading words, positive phrasing, single source of truth, pruning).
- A workspace already exists: `.agents/skills/notebrain-workspace/` with `evals/evals.json` (3 evals: kubernetes-reconciliation, kubernetes-architecture-graph, message-broker-backpressure), a `skill-snapshot/`, and `iteration-1/2/3` — iteration-3 is the latest (with `benchmark.json`/`benchmark.md` and `review.html`). The refs skill update runs as **iteration-4**.

## Decisions

1. **Parse the note file fresh; resolve the note via the index.** The index skips attachments by design (ast.go:105-110), so file parsing is required; the store is still the best slug/title resolver. Consequence: the command requires `--vault-path` and requires the note to be indexed (consistent with every sibling command). Vault-only resolution without an index is out of scope.
2. **Extraction lives in `internal/parser` as a new exported AST walker**, not regex in `cmd/`: reuses `mdParser`, correctly ignores links inside code fences, and handles aliases/anchors with existing helpers. Parser owns markdown semantics.
3. **Local attachments: wiki links AND markdown links.** Wiki syntax (`[[…]]` / `![[…]]`) per decision 7's resolution rules; markdown links and image embeds (`*ast.Link` / `*ast.Image`) whose destination is **not** an http(s) URL are also local attachment candidates, resolved note-folder-relative with percent-decoding and a vault-traversal guard.
4. **External links: three syntaxes, http/https only.** (a) markdown links and image embeds (`*ast.Link` / `*ast.Image`) whose `Destination` starts with `http://` or `https://`; (b) bare/angle autolinks (`*ast.AutoLink` with `AutoLinkType == AutoLinkURL`, `Protocol` `http`/`https`); (c) wikilink targets with `http://`/`https://` prefix. Excluded: email autolinks (`AutoLinkEmail`), `ftp://`, `mailto:`, `obsidian://` and other schemes. URL kept exactly as written; dedupe by exact string (no normalization).
5. **Kind taxonomy:** `image` (imageExts incl. `.heic`), `pdf` (`.pdf` — included here even though ingestion treats PDFs as notes), `other` (everything else in `attachmentExts`), `external-links` (URLs). Both embeds and plain links to attachments count; unknown extensions are notes, not attachments (allowlist already enforces this).
6. **Filters:** `--images`, `--pdf`, `--other`, `--external-links` — boolean, OR-combined; no flag = all kinds (files + URLs mixed). Audio/video stay under `--other` (no split in v1).
7. **Path resolution order:** wiki target contains `/` → vault-root candidate only (plus `./` → note-folder candidate); bare wiki target → note's folder, vault root, then `attachmentFolderPath` (first `os.Stat` hit wins; no hit → missing). Markdown destination → note's folder only, percent-decoded (`url.PathUnescape`), fragment stripped, and the resolved path must stay inside the vault (reject `..` escapes via `filepath.Rel`). External URLs: no filesystem resolution.
8. **Output:** absolute paths (agents must open them), with vault-relative `relative_path` alongside; external rows carry the URL in `path` with `relative_path` omitted and `missing` always `false` (see decision 11). JSON envelope `{"command":"refs","note_slug":…,"title":…,"total":N,"refs":[{"path":…,"relative_path":…,"kind":…,"missing":bool}]}`; TSV header `path\tkind\tmissing\trelative_path` (missing column empty for external); text one line per entry with `[kind]` label (`[external-links]` for URLs) and `(missing)` marker; empty result prints "No references found" in text mode only. `--jsonpath` works on the envelope. **Broken links are hidden by default**; `--include-missing` lists them marked `missing: true` — a quiet list beats noise, the opt-in flag keeps the information available.
9. **Deterministic ordering:** first-occurrence document order; dedupe in two layers — parser dedupes by cleaned target string, cmd dedupes by resolved absolute path (catches cross-syntax dupes like `[[a.png]]` + `[x](a.png)` pointing at the same file). URLs dedupe by exact string.
10. **PDF note input** (`file_path` ends in `.pdf`) → clear error ("is a PDF; refs are listed for markdown notes"). Indexed note whose file is gone from disk → error hinting `--vault-path` mismatch, not silent empty output.
11. **Offline by design: no network checks.** External links are never verified (no HTTP HEAD/GET); `missing` is `false` for them and `--include-missing` does not apply to them. Reachability checking would violate the tool's offline-first contract.
12. **Naming (settled by grilling):** command `refs`, JSON array `refs`, struct `RefsCmd` in `cmd/refs.go`, flag group `refs`, kind `external-links` (flag `--external-links`). "Refs" properly covers both local attachments and external URLs; it collides with nothing in the existing command tree.

## Scope

In scope: parser extractor (attachments via wiki + markdown syntax, external links) + tests, attachment-folder exposure + tests, `refs` command + tests, CLI registration, output formats, docs (README, wiki, skill ×4 files via the skill-creator loop, AGENTS.md, CHANGELOG).

Out of scope: frontmatter references (wiki/markdown syntax only), audio/video sub-filters, `obsidian://` and other non-http(s) URI schemes, email addresses, URL reachability checks (network), transitive (nested-note) reference listing, vault-only resolution without an index, references inside indexed PDFs.

## Tasks

- [x] **Task 0: branch — create `feat/refs-command`.**
  From clean master: `git switch master && git pull && git switch -c feat/refs-command`. Commit after each task with a Conventional Commit scoped to its package (`feat(parser): …`, `feat(ingest): …`, `feat(cmd): …`, `docs(wiki): …`, `docs(skill): …`). Push the branch (`git push -u origin feat/refs-command`) once Task 1 lands so work is never local-only. Do NOT merge to master — merging is the user's call after review.
  (**Seam:** n/a; **Files:** none (git only); **Verify:** `git branch --show-current` = `feat/refs-command`; `git status` clean before switching; `git log --oneline` shows the feature commits on the branch only.)

- [x] **Task 1: parser — extract refs (local attachments + external links) from a note body.**
  New file `internal/parser/attachments.go`: `type AttachmentKind string` (`KindImage`, `KindPDF`, `KindOther`, `KindExternalLinks`); `type AttachmentRef struct { Target string; Kind AttachmentKind }` (Target = cleaned — alias/anchor stripped for wiki refs, raw destination for markdown refs, full URL for external); `type ExtractedRefs struct { Attachments []AttachmentRef; External []string }`; `func ExtractReferences(body string) ExtractedRefs` — walks `mdParser`'s AST like `metadataTransformer` (ast.go:89-119), collecting:
  - attachments from wiki nodes: every `*wikilink.Node` whose target classifies as attachment (image/other via new classification that includes `.pdf`);
  - attachments from markdown nodes: `*ast.Link` and `*ast.Image` whose `Destination` is **not** http(s) and classifies as attachment (percent-encoding and fragments left raw here — resolution is the cmd layer's job);
  - external: `*ast.Link` and `*ast.Image` nodes with `Destination` starting `http://`/`https://`; `*ast.AutoLink` nodes with `AutoLinkType == ast.AutoLinkURL` and `Protocol` `http`/`https` (assemble `string(n.Protocol)+"://"+string(n.Text(src))` — protocol and text are stored separately, cf. renderer `html.go:506-515`); and `*wikilink.Node` targets with `http://`/`https://` prefix. Email autolinks, `ftp://`, `mailto:` and other schemes are skipped.
  Dedupe by Target (attachments) / exact URL (external), first-occurrence order. Reuses `imageExts`; adds `.pdf` handling alongside `attachmentExts`. `IsAttachmentLink`/`attachmentKind` untouched (ingestion semantics unchanged).
  (**Seam:** `internal/parser/parser_test.go` table tests; **Files:** `internal/parser/attachments.go`, `internal/parser/attachments_test.go`; **Verify:** `go test -count=1 ./internal/parser/` — wiki cases: `![[img.png|200]]`, `[[doc.pdf]]`, `[[img.png|alt]]`, `[[img.png#anchor]]`, `![[sub/img.png]]`, `[[./local.png]]`, code fence containing `![[x.png]]` (must not match), duplicate embeds (dedupe), unknown ext `[[archive.xyz]]` (not an attachment), `[[Note 1.2.3]]` (not an attachment), `.PNG` case-insensitivity; markdown cases: `![alt](img.png)`, `[doc](sub/file.pdf)`, `![alt](Router%20Modes.webp)` (raw destination kept), `[x](../up.pdf)`, `[x](STP.pdf#page=5)`, `[text](https://example.com)` (NOT an attachment — external), `[rel](../other-note)` (no ext, not an attachment), `#anchor`-only links (not an attachment), code fence containing `![x](img.png)` (must not match); external cases: `[text](https://example.com/a)`, `![alt](https://example.com/i.png)`, bare `https://example.com`, `<https://example.com>`, `www.example.com` (protocol `http`), `[[https://example.com]]`; excluded: `[mail](mailto:a@b.c)`, `[ftp](ftp://x.y/z)`, email autolink `a@b.c`, code fence containing `https://example.com` (must not match).)

- [x] **Task 2: ingest — expose the Obsidian attachment folder.**
  In `internal/ingest/ignore.go`, add `func LoadAttachmentFolderPath(vaultPath string) string` reading `.obsidian/app.json` (reuse the `ObsidianAppConfig` struct); refactor `LoadExcludedPaths` to share the read (keep its return value identical — non-regression guardrail per AGENTS.md). Returns `""` when absent/unreadable.
  (**Seam:** `internal/ingest/ignore_test.go`; **Files:** `internal/ingest/ignore.go`, `internal/ingest/ignore_test.go`; **Verify:** `go test -count=1 ./internal/ingest/` — folder read, absent file → `""`, existing `LoadExcludedPaths` tests still pass.)

- [x] **Task 3: cmd — `refs` command.**
  New file `cmd/refs.go`: `RefsCmd{ Note string (arg,`completion-predictor:"note-slug"`); Images, PDF, Other, ExternalLinks, IncludeMissing bool (group "refs") }`, help text "List a note's attachments and external links". `Run`: empty vault path → `UsageError` (message pattern of ingest.go:47-49); empty note → `UsageError`; `st.GetNoteMeta(ctx, c.Note)`; PDF note → error; read `filepath.Join(globals.VaultPath, meta.FilePath)` (missing file → error hinting `--vault-path`); `parser.ExtractReferences`; resolve each attachment ref (decision 7): wiki refs via candidate search + `os.Stat`; markdown refs via `url.PathUnescape(destination)`, fragment strip, `filepath.Join(noteDir, decoded)`, traversal guard (`filepath.Rel` must not escape vault); external URLs become `kind=external-links` rows directly (no stat, `missing` always false); dedupe by resolved absolute path; drop missing rows unless `--include-missing` (external rows unaffected); filter by kind flags (OR — `--external-links` selects only URL rows); sort stays first-occurrence; print via Task 4. Register `Refs RefsCmd` in `CLI` struct (cli.go:89-122).
  (**Seam:** `cmd` tests with the existing `fakeStore` (testhelpers_test.go:102 sets `f.noteMeta`) + real temp vault dirs via `t.TempDir()`; **Files:** `cmd/refs.go`, `cmd/refs_test.go`, `cmd/cli.go`; **Verify:** `go test -count=1 ./cmd/` — resolution from fake `noteMeta.FilePath`, bare wiki target found in note folder vs vault root vs attachment folder, wiki path target from vault root, `./` from note folder, markdown target from note folder (percent-decoded, e.g. `Router%20Modes.webp` → `Router Modes.webp`), markdown `../` traversal escaping vault rejected, cross-syntax dedupe (`[[a.png]]` + `[x](a.png)` → one row), missing file hidden by default, `--include-missing` shows it marked, filters (`--images` only, `--pdf` only, `--external-links` only, combined OR), external rows skip stat and always report `missing: false`, PDF-note error, empty-vault-path UsageError, note-not-found error passthrough.)

- [x] **Task 4: cmd — output formatting.**
  In `cmd/refs.go` (or `cmd/print.go`): `printRefsFormattedToWriter(w io.Writer, env, globals)` following `printTagsFormattedToWriter` (print.go:444-476) — text lines with `[kind]`/`[external-links]`/`(missing)` markers (empty → "No references found" line in text mode only), TSV header `path\tkind\tmissing\trelative_path` (external rows: empty missing and relative_path columns), JSON envelope (decision 8; external rows omit `relative_path`), `--jsonpath` via `printJSONPathResultToWriter`.
  (**Seam:** writer-based tests like `printTagsFormattedToWriter`; **Files:** `cmd/refs.go`, `cmd/refs_test.go`; **Verify:** golden-ish assertions per format + `--jsonpath='$.refs[*].path'`.)

- [x] **Task 5: docs — user-facing docs (README, wiki, AGENTS.md, CHANGELOG).**
  - `README.md`: new Features bullet after "Full Note Retrieval" (e.g. "**Reference Listing**: List a note's local attachments and external website links with `notebrain refs`, filterable by kind — images, PDFs, or URLs."); extend the Quick Start chaining example (step 6) with a `refs` snippet (e.g. `notebrain refs "$SLUG" --images --jsonpath='$.refs[*].path'`).
  - `wiki/Commands.md`: new `### refs` section after `get` (~line 256): usage, argument, flags, examples (`--images --format=json`, `--pdf --format=tsv`, `--external-links --format=json`), JSON shape.
  - `.agents/AGENTS.md`: add `cmd/refs.go` to the structure tree; add one line to CLI Testing & Flag Standards (positional `<note>`, `--images`/`--pdf`/`--external-links` filters).
  - `CHANGELOG.md`: add `## [Unreleased]` → `### Added` entry for the `refs` command.
  (**Seam:** none (docs); **Files:** the four listed; **Verify:** `grep -n refs` across the four files; no stale `attachments` command references and no stale flag names.)

- [x] **Task 6: skill — update `notebrain-assistant` via the skill-creator loop.**
  Update `.agents/skills/notebrain/` (SKILL.md + references/flags.md, example.md, schema.md) for the `refs` command, following the `skill-creator` skill's "Improving an existing skill" workflow (`/home/nimendra/.agents/skills/skill-creator/SKILL.md`) and the `writing-for-agents` craft rules (`/home/nimendra/.agents/skills/writing-for-agents/SKILL.md`; also read `SKILL-MECHANICS.md` before editing the frontmatter/description). Steps:
  1. Snapshot the current skill first: `cp -r .agents/skills/notebrain .agents/skills/notebrain-workspace/iteration-4/skill-snapshot/`.
  2. Edit the four files: SKILL.md **description** gains attachment/ref phrasing so queries like "list/fetch the images, files, attachments, or external links of a note" trigger it; the retrieval ladder gains a `refs` step (absolute paths, broken links hidden unless `--include-missing`, `--external-links` for URLs); flags.md gains the `### refs` flag table; example.md gains the two worked scenarios; schema.md gains the `refs` envelope + TSV columns. Craft rules: imperative voice, progressive disclosure (keep SKILL.md lean, references on demand), positive phrasing (no negations), single source of truth (verify every documented command against the real binary before writing it), leading words only where they earn their place.
  3. Extend `evals/evals.json` with refs-focused test prompts (e.g. "what images does my kubernetes note reference?", "fetch the attachments of kubernetes-architecture", "which external websites does the message broker note link to?") with assertions; keep the existing three evals unchanged.
  4. Run the eval loop as iteration-4 in `.agents/skills/notebrain-workspace/iteration-4/`: spawn with-skill vs old-skill-snapshot baseline runs in parallel per eval; capture `timing.json` per run; grade via `agents/grader.md`; aggregate via `python -m scripts.aggregate_benchmark <workspace>/iteration-4 --skill-name notebrain-assistant` (scripts in `/home/nimendra/.agents/skills/skill-creator/scripts/`); generate the review viewer headlessly with `eval-viewer/generate_review.py --static <output_path>` (no display in this environment; iteration-3 produced `eval_review_notebrain-assistant.html` the same way).
  5. Present results to the user, apply feedback, iterate (iteration-5 only if feedback demands).
  (**Seam:** eval workspace `.agents/skills/notebrain-workspace/`; **Files:** `.agents/skills/notebrain/{SKILL.md,references/flags.md,references/example.md,references/schema.md}` + `notebrain-workspace/iteration-4/`; **Verify:** `benchmark.md` shows the refs evals passing with-skill ≥ baseline; description triggers on attachment/ref phrasing.)

- [x] **Task 7: full verification.**
  `make test` (all packages), `make lint`, then a manual end-to-end against a real vault: `go build -o notebrain .` + `./notebrain refs "<note>"`, `--images --format=json`, `--jsonpath='$.refs[*].path'`, `--pdf` on a note linking `[[doc.pdf]]`, `--external-links` on a note with `[text](https://…)` links, a markdown-linked image (percent-encoded), and a deliberate broken link to confirm it is hidden by default and marked with `--include-missing`.
  (**Seam:** n/a; **Verify:** `make test && make lint`; manual outputs match decision 8.)

## Verification

- All work lands on `feat/refs-command`; `master` has zero commits from this feature until the user approves the merge (`git log master..feat/refs-command --oneline` lists exactly the feature commits).
- `go test -count=1 ./...` green (parser, ingest, cmd — store untouched, but run full suite).
- `notebrain refs "Note Title"` lists absolute paths; `--images`/`--pdf`/`--other`/`--external-links` filter; combined flags union.
- `--external-links` returns the note's http/https URLs (markdown links, image embeds, bare autolinks, `[[https://…]]`), deduped, first-occurrence order; email/ftp/relative links never appear; URLs inside code fences never appear.
- Markdown-linked local files resolve note-folder-relative, percent-decoded, traversal-guarded; `[[a.png]]` + `[x](a.png)` dedupe to one row.
- Broken links hidden by default; `--include-missing` lists them with `missing: true`; external rows never appear missing; no network I/O happens for external rows.
- JSON envelope matches decision 8 (`"refs": [...]`); `--jsonpath` works; TSV has the header row; text output marks `(missing)` and `[external-links]`.
- Skill docs (SKILL.md, flags.md, example.md, schema.md), README.md, wiki/Commands.md, AGENTS.md, CHANGELOG.md all mention the `refs` command with matching flag names.
- Task 6's iteration-4 benchmark.md shows the refs evals passing with-skill ≥ baseline, and the user reviewed the viewer output.

## Open Questions

- None — the design tree is fully settled (grilling rounds 1-2 complete).
- Non-blocking follow-ups (not part of v1): `--audio`/`--video` sub-filters (trivial enum extension), frontmatter reference scanning (needs per-field mapping rules), PDF page-anchor passthrough (`"anchor": "page=3"` in JSON), vault-only resolution without an index, and skill description trigger optimization via `scripts/run_loop.py` (skill-creator's Description Optimization step — 20 trigger queries; only if the user wants the description tuned beyond the manual wording update in Task 6).

---

# Plan: Cross-command flag & output consistency pass

All changes land on `feat/refs-command` (extending PR #37), after Task 7. Scope decision: **A + B + C all in this PR** (user decision — one review pass over a larger, but cohesive, diff).

## Goal

Eliminate flag/help-text confusion and output-format drift found by a full-codebase audit (3 exploration passes over `cmd/`, `internal/`, `internal/parser`, `internal/ingest`, `internal/store`). The `refs` feature introduced the worst offender — kind-filter flags advertised as additive ("include …") that actually restrict — and surfaced pre-existing inconsistencies across commands.

## Current State (audit findings, with file refs)

**Flag semantics — `refs` kind filters are restrictive but help says "include":**
- `cmd/refs.go:49-52` help: "include image attachments" (additive reading); `filterRefKinds` (`refs.go:243-265`): all-false → show everything; any-true → drop unselected kinds. Same command's `--include-missing` (`refs.go:53`) is truly additive → two semantic models in one command.
- `--images` (plural) vs kind value `"image"` (singular) and vs `--pdf`/`--other`/`--external-links` — plurality mismatch (`refs.go:49` vs kind table `refs.go:40-45`).

**Same concept, multiple flag names (PDF):**
- `ingest --enable-pdf` (`cmd/ingest.go:40`), `search --with-pdf` (`cmd/search.go:44`), `boosted --with-pdf` (`cmd/boosted.go:37`), `refs --pdf` (`cmd/refs.go:50`).

**Duplicate/alias flags:**
- `hidden --top-k` (deprecated) + `--candidate-chunks`; `--candidate-chunks 0` inexpressible (`hidden.go:45-48`). `--top-k` also means "chunks per note" in `search` (collision).
- `tags --for-note` alias of `--shared` (`tags.go:37`); global `--debug` alias of `--log-level=debug`; `--version` flag + `version` subcommand duplicate.
- `search --exclude-note` singular name for plural field `ExcludeNotes` (`search.go:45`).
- `get` positional `<SLUG>` vs `<NOTE>` everywhere else (`get.go:13` vs backlinks/connections/hidden/refs).

**Output drift:**
- `refs` TSV: external rows emit blank `missing` column while JSON emits `false` (`refs.go:287-289`).
- Search TSV header `slug` vs `get` TSV `note_slug` (`print.go:187` vs `get.go:59`).
- `get` TSV unescaped (raw tabs/newlines) vs escaped everywhere else (`get.go:59-62`).
- Score: JSON 4dp rounded; TSV `%f` 6dp (`print.go:119` vs `:190`).
- JSONPath: `get`/`stats` extract from bare structs — `$.command` fails; search/refs/tags extract from envelopes (`get.go:40`, `stats.go:49`).
- `--jsonpath`/`--format json` silently ignored by ingest/reset/doctor/init/version/completion.
- `command` envelope values decorated (`"connections --hops 2"`, `"hidden --deep --include-linked"`) — consumers must whitelist; `emptyResultHint` relies on `HasPrefix`.
- `tags --list` text mode prints `#tag\t(2 notes)` — tab in human output (`print.go:469`).

**Stringly-typed enum coupling (breakage risk):**
- `"pdf"` × 4 sites, 2 semantics: refs kind (`parser/attachments.go:21`, `cmd/refs.go:42`) vs `file_type` (`ingest/ingest.go:29`); kept apart only because refs refuses PDF notes (`refs.go:94-96`).
- `cmd/print.go:263` compares `r.FileType == kindPDF` (refs constant!) — works by coincidence.
- `"md"` literal at `store/query.go:1082` bypasses `fileTypeMD` (`ingest/ingest.go:28`).
- Vault-path UsageError duplicated verbatim (`ingest.go:49` == `refs.go:78`).
- Parser block-kind literals partially unconstantine (`ast.go:427` compares `"paragraph"` literally).
- Parser renderer labels non-image attachments `"attachment"` in chunk text vs `"other"` in refs — BUT stored chunk text change forces re-ingest; defer value change.

**Authored decision (user):** flag renames happen with working deprecated aliases (hidden from `--help`, still parse); TSV `missing` emits `false`.

## Decisions

1. **`refs` kind filters rename to `--only-*`** — `--only-images`, `--only-pdf`, `--only-other`, `--only-external-links`; help text states "limit to … (no filter = all kinds; combine to union)". Old flags (`--images`/`--pdf`/`--other`/`--external-links`) become hidden deprecated aliases, still functional. Satisfies both the semantics lie and the plurality mismatch.
2. **PDF family unifies on `--with-pdf`** for ingest (search/boosted already use it) via rename + hidden deprecated alias `--enable-pdf`; config key `enable-pdf` keeps working (alias field), `with-pdf` added to config.example.toml.
3. **`hidden --top-k` removed; `--candidate-chunks` becomes the single flag** (gets `default:"3"`). Eliminates the cross-command `--top-k` collision with search. Breaking — flagged in CHANGELOG.
4. **`get` positional → `<NOTE>`** (field rename `Slug` → `Note`); help text already matches siblings.
5. **`search --exclude-note` → `--exclude-notes`** with hidden deprecated alias field merged in Run.
6. **TSV `missing` = `false` for external rows** — delete the blanking special case (`refs.go:287-289`); matches JSON.
7. **TSV column parity:** search TSV header `slug` → `note_slug`; `get` TSV fields routed through `tsvEscape`; score printed at 4dp in TSV (same value as JSON).
8. **JSONPath envelope parity:** `get` and `stats` apply JSONPath to their full envelope (so `$.command` works). Breaking for existing paths like `$.note_slug` on get → `$.note.note_slug`; documented in CHANGELOG.
9. **Ignored `--jsonpath`/`--format` on text-only commands:** stderr warning, no output change.
10. **Enum hygiene:** `FileTypeMD`/`FileTypePDF` constants live in `internal/store` (or ingest-exported; decided at implementation — whichever avoids import cycles; store imports nothing from ingest, so constants in store and referenced from ingest + query.go + print.go); `print.go:263` drops the accidental `kindPDF` reuse; parser block-kind consts applied; vault-path UsageError shared constant. Parser renderer `"attachment"` label: **comment + const only, value unchanged** (would force re-ingest of every vault).
11. **`--has-tasks`/`--has-code` AND semantics:** documented in help text + wiki, no behavior change.
12. **`command` envelope decorated values + `tags --list` tab:** documented in wiki/Commands.md JSON schema section; no code change (human-facing text; stable-enough contract).

## Scope

In scope: refs flag renames + aliases + help text, TSV `missing` fix, ingest `--with-pdf` rename + config key, hidden `--top-k` removal, get `<NOTE>`, search `--exclude-notes`, TSV/JSONPath parity (`get`, `stats`, search family), ignored-flag warnings, enum constant hygiene, tests for every change, docs (README, wiki, AGENTS.md, CHANGELOG, skill ×4, config.example.toml).

Out of scope: `--debug`/`--for-note`/`--version` alias removals (harmless, users rely on them), parser renderer `"attachment"` value change, `command` envelope value normalization, tags text-mode tab, `--min-score` lexical-bypass bug (pre-existing behavior, separate fix), `--limit` default unification (tags 0 is intentional), `--skip-phantom` inverted default.

## Tasks

- [x] **Task A: `refs` kind filters → `--only-*` + TSV `missing` fix.**
  `cmd/refs.go`: rename fields to `OnlyImages/OnlyPDF/OnlyOther/OnlyExternal` with `name:"only-…"`; add hidden deprecated alias fields (`Images/PDF/Other/ExternalLinks`, `hidden:""`, help "deprecated: use --only-…"); `filterRefKinds` reads new|old; reword main help ("limit to …; no filter = all kinds; combine filters to union"). Delete the external-blank special case in TSV (`refs.go:287-289`). Verify kong `hidden:""` on flags still parses (fallback: keep visible with "(deprecated)").
  (**Files:** `cmd/refs.go`, `cmd/refs_test.go`; **Verify:** legacy flags still filter; hidden from `--help`; external TSV row shows `false`; `go test -count=1 ./cmd/`.)

- [x] **Task B: cross-command renames (PDF, hidden, get, search, ingest).**
  - `cmd/ingest.go`: `EnablePDF` → `WithPDF` (`name:"with-pdf"`); alias field `EnablePDF` hidden/deprecated still wiring `pipeline.EnablePDF`. `config.example.toml`: add `with-pdf`, keep `enable-pdf` with deprecation comment. `init.go` wizard wording check.
  - `cmd/hidden.go`: delete `TopK` + override merge; `CandidateChunks` gets `default:"3"`.
  - `cmd/get.go`: field `Slug` → `Note`.
  - `cmd/search.go`: `ExcludeNotes` gets `name:"exclude-notes"`; alias field `ExcludeNote` (name `exclude-note`) merged in Run.
  (**Files:** the four cmd files + their tests, `config.example.toml`; **Verify:** flags parse under new names; aliases work; hidden `--top-k` rejected with clear kong error; `go test -count=1 ./cmd/ ./internal/configfile/`.)

- [x] **Task C: output parity (TSV, JSONPath, ignored-flag warnings).**
  - `cmd/print.go:187`: `slug` → `note_slug`; score `%f` → 4dp rounded value (match JSON).
  - `cmd/get.go:59-62`: `tsvEscape` on text/tags/title columns.
  - `cmd/get.go:39-41` + `cmd/stats.go:48-50`: JSONPath against the envelope struct, not the bare NoteContent/Stats.
  - Also check refs TSV `kind` column: unescaped today (`refs.go:291` prints raw kind) — escape for parity or leave (enum-driven, safe); decide at implementation.
  - New warning: in `runMain` or per text-only command, if `globals.Format != formatText || globals.JSONPath != ""` → stderr warning that the command ignores them (ingest/reset/doctor/init/version/completion).
  (**Files:** `cmd/print.go`, `cmd/get.go`, `cmd/stats.go`, `cmd/cli.go` (+tests); **Verify:** TSV headers/escaping/score assertions updated; `$.command` works on get/stats JSONPath; warning appears on reset/doctor/ingest only once, stderr only.)

- [x] **Task D: enum hygiene + docs sync.**
  - Constants: `FileTypeMD`/`FileTypePDF` (home decided by import graph — likely `internal/store`); replace `"md"` at `store/query.go:1082`, `kindPDF` misuse at `cmd/print.go:263`, `.pdf`/`.md` suffix literals where a constant improves safety without churn.
  - Parser block-kind consts: `ast.go:339,357,370,427,439,442`.
  - Vault-path UsageError: shared constant (`ingest.go:49` + `refs.go:78`).
  - Parser renderer label `"attachment"`: const + comment only (no value change — re-ingest risk).
  - Docs: README.md:35,142, wiki/Commands.md:309-333, AGENTS.md flag-standards paragraph, CHANGELOG (`### Changed` renames table + `### Deprecated` aliases + breaking notes for `--top-k`, `--exclude-notes`, get JSONPath), skill files (SKILL.md:63,79, flags.md:83-87, example.md:27-28, schema.md:187-199 + TSV example row), `config.example.toml`.
  (**Files:** listed above; **Verify:** `grep` shows no stale flag names in docs; `golangci-lint run ./...` clean (goconst); `go test -count=1 ./...`.)

- [x] **Task E: full verification + review.**
  `make test`, `make lint`, rebuild binary (`make build` + `cp notebrain ~/.local/bin/`), manual probes: new flags, alias flags, hidden `--top-k` rejection, `--exclude-notes`, get `<NOTE>`, `--with-pdf` flag+config key, get/stats `$.command` JSONPath, TSV headers (search `note_slug`, refs external `false`, get escaped). Then `/code-review` over `master...HEAD` (standards + spec axes), apply fixes, route the final commit batch through the `git-commiter` skill in groups (feat/fix × code files, docs, config).
  (**Verify:** `git log master..HEAD` shows only feature+consistency commits; review findings resolved before merge; merge to master remains the user's call.)

## Verification

- `go test -count=1 ./...` green; `golangci-lint run ./...` 0 issues.
- `notebrain refs --help` shows `--only-*` with "limit to" wording, no `--images`/`--pdf`/`--other`/`--external-links`; legacy flags still parse and filter correctly (checked via CLI probe, not just unit test).
- `notebrain refs … --format tsv` external row: `false` in the `missing` column.
- `notebrain ingest --with-pdf` and config key `with-pdf` both work; `enable-pdf` config key still honored (deprecated).
- `notebrain hidden … --top-k 5` → kong error (flag removed); `--candidate-chunks` works.
- `notebrain search … --exclude-notes x` works; `--exclude-note` still accepted (deprecated).
- `notebrain get <note> --jsonpath='$.command'` → `get`; `--jsonpath='$.note.note_slug'` works.
- TSV: search header `note_slug`; get TSV survives multiline/tab text (single parseable row); scores at 4dp.
- `notebrain reset --format json` warns on stderr; stdout unchanged.
- Docs: zero stale references to old flag names (`rg -- '--images|--enable-pdf|--top-k|--exclude-note|SLUG'` across README/wiki/AGENTS.md/skill/CHANGELOG/config.example.toml, allowing explicit "deprecated:" notes).
- Skill evals 17/18 (refs scenarios) use `--only-*` names only if the fixture asserts flag names — regenerate fixture-based assertions if needed, rerun affected gradings; iteration-4 benchmark stays green.

## Open Questions (non-blocking)

- Long flag `--only-external-links` — accepted as-is (matches the kind value `external-links`; no shortcut alias in v1).
- Parser renderer `"attachment"` label — value change deferred forever unless a future `chunkSchemaVersion` bump justifies re-ingest; documented via const + comment. (This decision matches the earlier draft; no change unless user insists.)
