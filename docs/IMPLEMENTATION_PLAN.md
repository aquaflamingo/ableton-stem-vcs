# Implementation Plan — Stem

Owner: engineering · Date: November 10, 2026
Design: [TDD-v0.2.md](TDD-v0.2.md) · Critique: [CRITIQUE.md](CRITIQUE.md) · Evidence: [FORMAT-NOTES.md](FORMAT-NOTES.md)

This plan turns TDD v0.2 into buildable work. It assumes Go 1.27+ (single static
binary, fast streaming XML, easy cross-compile to macOS/Windows), one git
repository per Live project, and Git LFS for assets.

## Guiding principles

1. **Lossless first.** The canonical tree must reproduce the Set. Raw numeric text
   and unknown XML are preserved verbatim. (NFR1)
2. **SIDs are the only merge-time identity.** Live Ids are assigned only at implode.
3. **Whole-tree operations, not Git drivers.** `stem merge`/`rebase`/`cherry-pick`
   own merging; Git merge attributes are a safety net only.
4. **Fidelity is provable, acceptance is tested by Live.** Canonical round-trip is
   CI-testable; "Live opens it" needs a real Live host.
5. **Ship the boring win first.** The gunzip textconv fallback (M1) is useful in
   week one and shares the architecture.

## Target repository layout (Go)

```
stem/
  go.mod
  cmd/stem/                 # CLI entrypoint (+ `stem implode` subcommand; standalone binary)
  internal/
    als/                    # .als I/O: gzip detect, decode/encode, XML
    adapter/
      live11/               # Live 11 element tables, ID spaces, generators
      live12/               # later
      registry.go           # version -> adapter from root attrs
    model/                  # entity graph, SID, ID spaces, parent/child, sends map
    canon/                  # explode (tree <- graph) and implode (graph <- tree)
    identity/               # Id->SID map, fingerprint matcher, match log
    diff/                   # semantic change list, terminal + JSON renderers
    merge/                  # three-way structural merge, sequence merge, validation
    assets/                 # content addressing, manifest, path rewrite, mtime/CRC restore
    plugins/                # opaque blob sidecars, plugin manifest, Max device handling
    gitx/                   # git plumbing: trees, refs, hooks, CAS lock refs, LFS
    watcher/                # filesystem watch, autosave refs
    reviewui/               # local web timeline diff + bounce playback
  docs/                     # this doc set
  corpus/                   # real projects (gitignored), README committed
  testdata/                 # golden fixtures, synthetic Sets
  scripts/                  # dev helpers, corpus fetch, live-smoke runner
```

### Key type sketches

```go
type SID string // UUID

type Entity struct {
    SID     SID
    Kind    string          // "track","device","clip","automation.lane",...
    Parent  SID
    LiveID  map[string]int  // last-known Id per ID space, for identity matching only
    Order   int
    Attrs   map[string]string
    Raw     []byte          // exact XML fragment, unknown children included
    Children []*Entity
}

type Set struct {
    Root     map[string]string // version attrs
    Entities map[SID]*Entity
    Order    map[string][]SID  // collection name -> SID order
    Sends    map[SID]map[SID]  // track SID -> return SID -> send value
}
```

## Phase 0 — Spike (3–4 weeks)

**Goal:** remove the unknowns that could invalidate the design. Mostly experiments,
some throwaway code. Output is a written findings report extending
[FORMAT-NOTES.md](FORMAT-NOTES.md).

### Experiments and pass/fail

| # | Question | Method | Pass criterion |
| --- | --- | --- | --- |
| E1 | Is Live's XML byte-stable on no-edit re-save? | Save the same Set twice; compare decompressed bytes | Report result; drives CH-A5 |
| E2 | Are Ids stable across restart/collect-all? | Save, quit, reopen, re-save; diff all Id spaces | Pre-existing Ids unchanged, or characterize drift |
| E3 | Does mtime/CRC make Live flag samples? | Checkout with/without mtime restore; observe | Determines CH-B5 treatment |
| E4 | Do unknown attributes survive load+save? | Inject a bogus attribute; open+save; check | Either survives, or we must drop unknown-attr pass-through claim |
| E5 | Warp/analysis authority (.als vs .asd) | Delete `.asd`, open, observe re-analysis | Determines exclusion policy |
| E6 | **Merge feasibility** | Hand-build branch A (+track) and B (+return track); implode, merge, open in Live | Live opens merged Set with both edits (CH-D2) |
| E7 | Explode→implode→open fidelity | On ≥5 real projects | Live opens all; no prompts |
| E8 | XML library choice | Benchmark streaming parse of the 5.1 MB sample | Fit within NFR2 budget with headroom |

**Requires a licensed Live host (macOS or Windows).** E1–E7 cannot run on the
Linux dev box; plan for a dedicated smoke host (see Test infrastructure).

**Exit criteria:** findings report committed; design updated if E1/E4/E6 change an
assumption; a synthetic corpus generated.

## Phase 1 — Versioning (6–8 weeks)

Ship a tool a producer can use daily for one-way versioning.

- **M1a Gunzip fallback (week 1–2).** `stem commit` stores the raw decompressed XML;
  `textconv` gives line diffs; a `stem raw` mode works with no entity model. Shares
  asset store + git plumbing. (CH-D4)
- **M1b `.als` codec.** `internal/als`: gzip detect, stream decode, entity-preserving
  XML. Golden round-trip fixture: `parse → re-encode == input bytes` for the corpus
  (byte-exact re-encode before any canonicalization).
- **M1c Canonical model + explode/implode** for Live 11. SID assignment, ID spaces,
  sends map, `order.json`, `identity.json`. Unknown XML preserved verbatim.
- **M1d Asset manager.** LFS integration, manifest (metadata only), mtime/CRC
  restore, path rewrite, collision handling at implode time. (CH-C2, CH-C1, CH-B5)
- **M1e `doctor`.** Missing samples, external refs, plugin manifest, Live version,
  frozen-track audit. (FR5)
- **M1f Git plumbing.** Explode-on-commit, implode-on-checkout (with Live-open
  detection and sibling-file fallback), hidden backup ref, hooks.
- **M1g Standalone `implode`.** Static binary, no dependencies. (CH-D1)

**Exit criteria:** author uses it on real songs for 4 weeks with no data loss;
`implode` reproduces a Set from a fresh clone; round-trip fixed point in CI.

## Phase 2 — Semantic diff (4–6 weeks)

- Typed change list per TDD v0.2 §7 (tracks, groups, clips, MIDI notes, devices,
  params, automation, plugin blobs, samples, tempo, returns/sends).
- Terminal renderer, JSON renderer, `log --entity`.
- Minimal Review UI: arrangement timeline, colored regions; bounce playback stub.
- **Exit criteria:** 5 producers rate corpus diffs readable.

## Phase 3 — Merge (8–10 weeks)

- `internal/merge`: three-tree structural merge (TDD v0.2 §9), property-by-property,
  SID-keyed sequence merge with deterministic tie-breaks. (CH-C6)
- Referential validation gate; abort-to-conflict state.
- `stem merge` / `rebase` / `cherry-pick` over commit trees (not drivers).
- Conflict UX (semantic entries + non-interactive flags).
- Identity hardening: recorded-map primary, fingerprint fallback, `stem rematch`,
  match log. (CH-A7, CH-B3)
- **Exit criteria:** two-person trial, 20 merges, zero broken Sets.

## Phase 4 — Collaboration (6 weeks)

- CAS lock refs + LFS locking; advisory/strict modes. (CH-C4)
- Soft ownership (CODEOWNERS-style).
- PR integration (semantic summary comment), reference bounces in LFS.
- Review UI audio before/after compare; comments anchored to bar ranges/tracks.
- **Exit criteria:** a team of 4 completes an EP on Stem.

## Phase 5 — Hardening

- Multi-version adapters (Live 12); Windows parity; perf work; docs.
- Optional Max for Live status device.
- **Exit criteria:** public beta.

## Test infrastructure

| Layer | Runs on | Tooling | Cost |
| --- | --- | --- | --- |
| Unit + golden round-trip | Linux CI | `go test` | none |
| Property tests (merge) | Linux CI | `testing/quick` or `gopter` | none |
| Fuzzing (.als parser) | Linux CI | `go test -fuzz` | none |
| Corpus round-trip matrix | Linux CI | CI matrix over corpus versions | corpus storage |
| **Live smoke (open + re-save)** | **macOS + Windows hosts** | UI automation + screenshots | **licenses + host + maint.** (CH-D3) |
| Merge property + idempotence | Linux CI | generated edits + invariants | none |

The Linux box cannot run Live. Treat the Live smoke host as a first-class piece of
infrastructure with its own budget and a documented runbook (`scripts/live-smoke`).

## Corpus plan

- Current: one real Live 11.3.43 set (`corpus/supreme-something/`, gitignored),
  with a backup autosave for identity/ordering comparisons.
- Needed: small/medium/huge; session-heavy, arrangement-heavy, rack-heavy;
  plugin-heavy (VST2/VST3/AU — the current sample has none); Max-for-Live-heavy;
  at least one Live 12 set; multi-group with many returns.
- Collection: owner permission required; store hashes and a manifest; do not commit
  audio to the public repo.
- Synthesizer: generate Sets programmatically for exhaustive/fuzz cases (many
  tracks, deep racks, dense automation).

## Risk register (build-relevant)

| Risk | Mitigation in the plan |
| --- | --- |
| Live rejects an imploded Set | Byte-exact re-encode first (M1b); verbatim unknown XML; hidden backup ref; smoke tests |
| Ordering assumption wrong | E1 is the first experiment; `preserve_source_order` default (CH-A5) |
| Merge model can't merge returns/sends | E6 proves the hard case in Phase 0 (CH-B1, CH-D2) |
| Float rounding breaks fidelity | exact-text numerics in canonical form (CH-A4) |
| Missing Live host blocks progress | Phase 0 E1–E7 scheduled around host availability; synthetic corpus unblocks diff/merge work |
| Windows path issues | SID-based paths from day one (CH-A3) |

## Definition of done (per phase)

Each phase ends with: code merged to `main`, CI green (unit + property + fuzz),
docs updated, `REPO_MAP.md` updated, a written exit-criteria report, and — where
Live is involved — smoke-test artifacts (screenshots + re-saved Sets) attached.
