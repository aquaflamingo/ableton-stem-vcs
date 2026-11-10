# Stem — project instructions

Read [README.md](README.md) first, then the docs in this order:
`docs/CRITIQUE.md` → `docs/TDD-v0.2.md` → `docs/IMPLEMENTATION_PLAN.md`.

## Ground rules

- **Design docs are the source of truth.** `docs/TDD-v0.2.md` supersedes
  `docs/TDD-v0.1.md`; keep v0.1 verbatim for history. If code and docs disagree,
  fix one deliberately and say which.
- **Lossless is the contract.** The canonical form must reproduce the `.als`.
  Preserve raw numeric text and unknown XML verbatim. Never round floats in the
  canonical bytes. (NFR1, CH-A4)
- **SIDs are the only merge-time identity.** Live numeric Ids are (re)assigned only
  at implode, never during merge. (CH-A6)
- **Merges are whole-tree operations** in `stem`, not Git merge drivers. Git's
  per-file merge is a safety net only. (CH-A2)
- **Do not commit corpus contents.** `corpus/*/` is gitignored on purpose; these are
  real songs. Only `corpus/README.md` is tracked. (See `corpus/README.md`.)

## Language and tooling

- Go 1.27+, stdlib-first; keep dependencies minimal. Prefer `encoding/xml` streaming
  unless benchmarks (Phase 0 E8) show it is too slow.
- One static binary, cross-compiled for macOS and Windows.
- CI: unit + golden round-trip + property + fuzz on Linux. Live smoke tests run on
  separate macOS/Windows hosts (`scripts/live-smoke`, Phase 0+).

## Conventions

- Package layout is fixed by `docs/IMPLEMENTATION_PLAN.md` (`internal/als`,
  `adapter`, `model`, `canon`, `identity`, `diff`, `merge`, `assets`, `plugins`,
  `gitx`, `watcher`, `reviewui`).
- No comments in code unless they explain a non-obvious decision.
- Update `/root/workspace/REPO_MAP.md` whenever files are added, moved, or deleted.

## Verification

- `go build ./...` and `go test ./...` must pass before any change is considered done.
- Round-trip fixtures must stay byte-exact at the codec layer and fixed-point at the
  canonical layer.
- Run `gofmt`/`go vet`. If golangci-lint is configured, run it too.

## When Live is involved

Live cannot run on the Linux dev box. Anything that needs "Live opens it" must go
through the smoke host and produce artifacts (screenshots + re-saved Sets). Never
claim Live acceptance from a Linux-only test.
