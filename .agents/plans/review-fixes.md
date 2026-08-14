# Plan: Post-release review fixes (v2.12.0 → HEAD)

Status: **in progress** — approved 2026-08-14, implementation underway.
Sources: agy-delegate review run (gemini-3.1-pro-high, read-only) + code-review skill
(Standards + Spec axes) over `git diff v2.12.0...HEAD` (40 commits, 77 files).

User selection: fix items 1–9 + 11; skip #10 (Plan.md drift — the refs text
styling/relative-path deviation was user-requested post-plan, sanctioned).

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