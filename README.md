# Stem

Version control and collaboration for Ableton Live projects.

Stem treats a Live project folder the way Git treats a source tree. A Live Set
(`.als`) is decomposed into a deterministic, diffable, mergeable canonical form so
that contributors can work on branches, review semantic diffs, and merge with
entity-level conflict resolution instead of whole-file XML conflicts.

Stem is a **layer on top of Git**, not a replacement: it uses Git for
content-addressed storage, history, remotes, and review, and adds a lossless
`.als` codec, a semantic diff engine, a whole-tree three-way merge, and asset /
plugin handling.

> Status: **prototype (v0.2)**. The `.als` codec, canonical explode/implode, and a
> semantic diff work end-to-end on a real Live 11.3.43 Set. No merge or git
> integration yet. See [docs/SPIKE-2026-11.md](docs/SPIKE-2026-11.md).

## Documentation

| Document | What it is |
| --- | --- |
| [docs/TDD-v0.1.md](docs/TDD-v0.1.md) | The original draft design, preserved verbatim. |
| [docs/CRITIQUE.md](docs/CRITIQUE.md) | Engineering review of v0.1: blockers, missing cases, fixes. |
| [docs/TDD-v0.2.md](docs/TDD-v0.2.md) | The revised design that applies the critique. |
| [docs/IMPLEMENTATION_PLAN.md](docs/IMPLEMENTATION_PLAN.md) | Phased build plan, package layout, experiments. |
| [docs/FORMAT-NOTES.md](docs/FORMAT-NOTES.md) | Measured facts from a real Live 11.3.43 Set. |
| [docs/SPIKE-2026-11.md](docs/SPIKE-2026-11.md) | Base prototype: what was built, results, next steps. |

Start with **CRITIQUE.md** for the review, then **TDD-v0.2.md** for the design,
then **IMPLEMENTATION_PLAN.md** for how it gets built, then **SPIKE-2026-11.md**
for what already runs.

## Prototype

```
go build -o stem ./cmd/stem
stem check "corpus/supreme-something/supreme something.als"   # byte-exact + fixed point
stem explode "$SET" /tmp/canon
stem implode /tmp/canon /tmp/rebuilt.als
stem diff "$SET" "$SET2"
go test ./...
```

The codec round-trips the real 5.1 MB Set byte-for-byte, and the canonical form is
a fixed point. A 5.36 MB Set parses in ~21 ms (~255 MB/s).

## The core idea in three lines

1. A `.als` is a ~10:1 gzip-compressed XML blob with many independent ID spaces and
   opaque state — plain Git can only diff it as bytes.
2. Stem explodes it into a canonical, SID-addressed tree that Git can diff and merge,
   and implodes it back into an openable Set on checkout.
3. Assets go through Git LFS; opaque plugin/Max state is stored byte-exact.

## Target

Go 1.27+, single static cross-platform binary (macOS, Windows), Git LFS for audio.

## Corpus

See [corpus/README.md](corpus/README.md). Real `.als` projects are **not** committed
(they may contain unreleased music); only the README is.
