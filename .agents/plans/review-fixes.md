# Plan: Post-release review fixes (v2.12.0 → HEAD)

Status: **complete** — all items landed 2026-08-14 (13 commits, see below); full suite + lint green.
Sources: agy-delegate review run (gemini-3.1-pro-high, read-only) + code-review skill
(Standards + Spec axes) over `git diff v2.12.0...HEAD` (40 commits, 77 files).

User selection: fix items 1–9 + 11; skip #10 (Plan.md drift — the refs text
styling/relative-path deviation was user-requested post-plan, sanctioned).

## Landed commits (in order)

1. `chore(plan)`: record this plan (0bbbc5b)
2. `refactor(store)`: unexport Levenshtein and ClosestTags (32b6a00) — items 1
3. `refactor(cmd)`: drop unused Globals param, share JSON envelope printing (1014084) — items 2+7 (print.go merged: hunk staging misaligned, merged into one focused single-file commit)
4. `test(cmd)`: cover get/stats JSONPath envelope (a5c977d) — item 3
5. `refactor(cmd)`: rename topK local to candidateChunks in hidden deep mode (6d1a585) — item 4
6. `refactor(cmd)`: type refs kinds via parser constants, group resolution in refResolver, drop deprecated alias flags (6204eb9) — items 6+9+10 (hunk-entangled in refs.go: Kind field typing, const deletion, and alias-field removal share adjacent regions; kept as one coherent refs refactor; tests updated in the same commit)
7. `refactor(cmd)`: use shared JSON envelope printing in get, stats, refs (47e80a7) — item 7 remainder
8. `refactor(cmd)`: inline lexicalFallback middle man (cf883c4) — item 8
9. `refactor(cmd)!`: remove deprecated --enable-pdf flag and enable-pdf config key (128e251) — item 11a
10. `refactor(cmd)!`: remove deprecated --exclude-note flag (a0fe047) — item 11b
11. `refactor(cmd)!`: remove legacy --debug flag (c1eb82d) — item 11c
12. `refactor(config)!`: remove deprecated hide-tags config key (a97bbb5) — item 11d
13. `docs`: purge removed flag aliases from docs and changelog (ab28f86) — docs sweep

## Deviations from plan (approved scope, noted for review)

- **Item 5 (groupCompletion) — no change**: the constant is a live `textOnlyCommands`
  map key, and inlining it trips `goconst` (3 occurrences). Kept as-is; finding was a
  false positive.
- **Items 6+9+10 merged** into one refs.go commit (hunk-level entanglement; splitting
  risked mid-state commits that cannot compile — Kind field typing, const deletion,
  and alias removal share adjacent hunks).
- **Items 2+7 merged** for print.go (hunk staging misalignment).
- **Docs updated in the final docs commit** (ab28f86) instead of per-code-commit.
- Tests for removed aliases became rejection tests (parse must error); `TestRefsOnlyFlagsParse`,
  `TestIngestWithPDFFlag`, `TestSearchExcludeNotesFlagRename` restructured accordingly.

## Verification (all green)

- `make test` (10 packages), `make lint` (0 issues), `make build`.
- CLI probes: `refs --help` shows only `--only-*`; `ingest --help` only `--with-pdf`;
  `search --help` only `--exclude-notes`; global help only `--log-level`;
  `--enable-pdf`/`--exclude-note`/`--debug`/`refs --images` all rejected with usage errors.
- Docs sweep: zero stale alias references (rg across README/wiki/AGENTS.md/skill/CHANGELOG/config.example.toml;
  CHANGELOG keeps historical entries, Unreleased Deprecated section now empty).
- `hide-tags` config key now ignored (test asserts ShowTags stays false).

## Commits (small commit per change, in order; each standalone-buildable)

1. `refactor(store): unexport Levenshtein and ClosestTags`
   - tags.go: `ClosestTags`→`closestTags`, `Levenshtein`→`levenshtein` (+ internal callers :63, :83)
   - tags_test.go:124-139 update

2. `refactor(cmd): drop ignored Globals param from printDeepDetails`
   - print.go:339 signature; caller print.go:306 (no test callers)

3. `test(cmd): cover get/stats JSONPath envelope`
   - output_parity_test.go: GetCmd/StatsCmd via fakeStore — `--jsonpath='$.command'` → get/stats; get `$.note.note_slug`

4. `refactor(cmd): rename stale topK local in hidden deep mode`
   - hidden.go:44 `topK`→`candidateChunks` (used at :65)

5. `refactor(cmd): inline single-use groupCompletion constant`
   - cli.go:44 const removed; map key literal `"completion"` at :217

6. `refactor(cmd): use parser.AttachmentKind constants in refs`
   - delete refs.go:44-49 local consts (values identical); `Kind: string(parser.KindExternalLinks)`;
     comparisons/chip-style/filter switch on parser.Kind*; drop "independence" comment;
     tests switch to parser.Kind*; JSON/TSV output unchanged

7. `refactor(cmd): share JSON/JSONPath envelope printing`
   - new `printJSONEnvelope(w, env, globals) (bool, error)` in print.go
   - refactor get.go:49-57, stats.go:56-64, refs.go:307-315, print.go:452-460 (tags)
   - JSONPath first, then formatJSON indented encode — byte-identical output

8. `refactor(cmd): inline lexicalFallback middle man`
   - search.go:177 method → direct st.LexicalSearch at :238, :266

9. `refactor(cmd): group refs path context into refResolver`
   - struct {vaultPath, noteDir, attachmentFolder}; methods resolveAttachment/wikiCandidates/insideVault/vaultRelativePath
   - no direct test callers (verified)

10. `refactor(cmd)!: remove deprecated refs kind aliases` (BREAKING)
    - refs.go fields Images/PDF/Other/ExternalLinks + merge logic
    - drop deprecated subtests (refs_test.go:136-153, :543-550)
    - docs: AGENTS.md:79, flags.md:89, wiki/Commands.md:315; CHANGELOG Deprecated→Removed

11. `refactor(cmd)!: remove deprecated --enable-pdf and enable-pdf key` (BREAKING)
    - ingest.go:41 field + `c.WithPDF || c.EnablePDF`→`c.WithPDF`
    - init.go:19+76 prefill `existing.WithPDF || existing.EnablePDF`→`existing.WithPDF`
    - config.example.toml:69-70 deprecated lines
    - ingest_flags_test.go legacy subtest; docs AGENTS.md:79, wiki/Commands.md:163; CHANGELOG

12. `refactor(cmd)!: remove deprecated --exclude-note` (BREAKING)
    - search.go:46 field + merge :115-117
    - search_flags_test.go legacy; docs wiki/Commands.md:219, flags.md:24; CHANGELOG

13. `refactor(cmd)!: remove legacy --debug flag` (BREAKING)
    - cli.go:74 field; setupLogging/resolveLogLevel drop debug param (:303, :350-357)
    - cli_test.go:34-35; config.example.toml:30-31
    - docs wiki/Commands.md:19,30, flags.md:123, AGENTS.md:79; CHANGELOG

14. `refactor(config)!: remove hide-tags deprecated key` (BREAKING)
    - toml.go:17 map entry + negation branch :83-87
    - toml_test.go:252; CHANGELOG

## After implementation

- `make test` (full suite, 10 packages) + `make lint` + `make build`
- Docs + skill validity: `rg` removed tokens (`--enable-pdf`, `enable-pdf`, `--exclude-note`,
  `--debug`, `--images`/`--pdf`/`--other`/`--external-links`, `hide-tags`) across
  README.md / wiki/ / .agents/ / skill refs / config.example.toml; fix stragglers in a final
  docs commit; CLI probe refs/ingest/search `--help` for alias remnants

## Notes

- Plan.md:223 previously declared `--debug`/`hide-tags` removals out-of-scope — user instruction
  overrides; both removed as breaking changes (pre-v2.13, Unreleased window).
- Skip item #10 (Plan.md drift) per user selection.