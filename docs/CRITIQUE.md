# Critique of Stem TDD v0.1

Reviewer: engineering review · Date: November 10, 2026
Baseline: [TDD-v0.1.md](TDD-v0.1.md) · Evidence: [FORMAT-NOTES.md](FORMAT-NOTES.md)

## Verdict

The product thesis is sound: an `.als` is a compressed, order-unstable, id-fragile
XML blob, and treating a Live project like a source tree is the right idea. The
problem is that the **Git integration described in §5 and §9 does not match how
Git's extension points work**, and several **canonicalization rules in §6
contradict the losslessness requirement (NFR1)**. Most of these are fixable on
paper. The revised design is in [TDD-v0.2.md](TDD-v0.2.md).

Severity legend: **Blocker** = cannot ship as written · **High** = will cause data
loss or rework · **Medium** = quality/UX · **Low** = polish.

## A. Structural flaws

### CH-A1 — Clean/smudge filters cannot explode one file into many · Blocker
§5 lists clean and smudge filters as the integration mechanism. A Git filter is a
1:1 transform on a single path's content. It cannot turn `MySong.als` into the
`set/tracks/...` tree of §6, because the stored form is one blob per file and the
exploded form has hundreds of files.

The design actually implies: `*.als` is a **gitignored build artifact**, `stem
commit` explodes it, and implosion happens on checkout via a hook or an explicit
`stem checkout`. §6 already assumes this ("the .als in the working tree is a build
artifact … rebuilt on checkout") but §5 contradicts it. Pick the §6 model.

**Fix:** state that `.als` is ignored; explode on commit, implode on checkout.
Ship a **static `stem implode` binary** so a plain `git clone` + that binary can
reproduce the Set. This also softens NFR4 (see CH-D1).

### CH-A2 — Git merge drivers are per-file; the merge in §9 is whole-tree · Blocker
§9 registers "a Git merge driver for the canonical tree." A merge driver is
invoked once per conflicting file with three *files*. But the algorithm needs the
whole tree at once:
- step 4 reorders collections that live in different files (track order);
- step 5 rewrites Ids and references that cross files (automation targets, routing);
- step 6 validates global referential integrity.

Worse, Git decides which directories correspond by **rename detection**, not by
SID. A track whose directory is renamed (see CH-A3) looks like a delete+add to
Git, so the driver never sees the rename.

**Fix:** `stem merge` / `rebase` / `cherry-pick` perform the three-way merge
**themselves** from the commit graph (read the three trees, e.g. via
`git merge-tree` or by reading `base/ours/theirs` trees directly), write the merged
tree, and create the commit. The per-file `merge=stem` driver degrades to a
conservative conflict marker for the case where plain Git is used. §9's algorithm
becomes a library function over three in-memory trees, not a driver.

### CH-A3 — Paths encode order and name, which fights SIDs and Windows · High
§6 uses `tracks/0007-drums/` and `devices/01-drum-rack.xml`. Because §6 also
promises SID-based identity (moves/renames must be detectable), encoding the
human name and position in the path guarantees that **renaming or reordering a
track rewrites every path under it**. That defeats move detection, floods history,
and creates merge noise — the exact problem SIDs exist to solve. It also invites
Windows failures (illegal characters, trailing dots/spaces, case-insensitive
collisions, `MAX_PATH`).

**Fix:** paths keyed by SID (`tracks/<sid>/devices/<sid>.xml`), names and order
carried as data, rendered only in diff/UI output. §9 step 5 even says "in the SID
model this is automatic" — lean into it.

### CH-A4 — "Fixed precision for floats" is lossy and breaks NFR1 · High
§6 canonicalization says "fixed precision for floats, no trailing zeros." A Live
Set is full of float parameters (`Gain`, `Freq`, `Value`). Rounding them is a
lossy transform, so explode→implode→open cannot reproduce the original Set and
NFR1's fixed point is false. Live may also re-serialize a rounded value
differently, breaking the "identical canonical form" half of NFR1.

**Fix:** store the **exact text** of every numeric as written; normalize only for
display and for comparison (e.g. parse-to-float then format for the diff view,
but round-trip raw text). Keep a separate canonical hash used for equality.

### CH-A5 — Blind child re-sorting contradicts unknown-element passthrough · High
§6 sorts children by a stable key; §13 promises to "preserve unknown XML
verbatim." These conflict: reordering children the tool does not understand can
change what Live loads. And the premise ("ordering not guaranteed stable across
saves") is **not yet proven** — [FORMAT-NOTES.md](FORMAT-NOTES.md) §6 shows no
measurement because our two sample saves differ for real reasons.

**Fix:** (a) measure ordering stability first with a controlled double-save;
(b) only reorder collections that are *known* unordered, and only those whose
order Live ignores; (c) keep original order everywhere else. If ordering turns out
stable, do not reorder at all.

### CH-A6 — §9 contradicts itself on ID reconciliation · High
§9 step 5 says "allocate fresh IDs for one side and rewrite all references," then
in the same sentence says "in the SID model this is automatic." Those are two
different designs. The first edits Live Ids during merge; the second never does
and assigns them at implode. Running both means an entity can get renumbered twice.

**Fix:** delete the renumbering branch. SIDs are the only merge-time identity; all
Live Ids and counters (`NextPointeeId`, per-list next-Id) are (re)generated only
by implode, deterministically. See CH-N3.

### CH-A7 — SID matching ignores the strongest signal it owns · High
§6.1 matches entities by `(Live Id, type, parent)` "where stable," then falls back
to structural fingerprints under the same parent. But Stem **assigns the Live Ids
itself at implode** — it wrote a Live-Id→SID map moments earlier. Discarding that
map and re-deriving identity from scratch makes the highest-risk algorithm
(§12: "highest-risk algorithm in the system") needlessly hard.

**Fix:** persist the Id→SID map Stem wrote, in `.stem/` per checkout (and
optionally in a committed, human-readable `identity.json`). On ingest, prefer the
recorded map; fingerprint matching is only for entities Live *created* since the
last checkout. [FORMAT-NOTES.md](FORMAT-NOTES.md) §2 shows Live keeps Ids stable,
which makes the map exact for existing entities in the common case.

## B. Missing domain cases

### CH-B1 — Return tracks mutate every track (sends) · High
Adding a return track adds a `TrackSendHolder` to every track's mixer. Two
branches that each add a return will conflict on *every* track if sends are a
positional list. [FORMAT-NOTES.md](FORMAT-NOTES.md) §1 shows 160 `TrackSendHolder`
elements for 20 tracks (8 sends each).

**Fix:** model sends as a **map keyed by the return's SID**, not a positional list.
Then disjoint return additions merge cleanly. Document returns as an ordered
collection in their own right (return Ids were `2,34…41,49` in the sample).

### CH-B2 — Group tracks, racks and Drum Rack pads · High
The sample has 2 `GroupTrack`s (Id 60 `BUS_KICK`, Id 59 `perc`) with children.
§6/§9 treat tracks as a flat list. Grouping/folding a track on one side while the
other edits it, nested Rack chains, macro→parameter mappings, and Drum Rack pads
(addressed by MIDI note) all need explicit modeling.

**Fix:** add a track **parent/child** relation to the model and a tree-aware
sequence merge; treat Rack chains and Drum-pad maps as ordered collections keyed
by SID; add a conflict rule for "reparented vs edited."

### CH-B3 — Numeric-Id stability is better than assumed; the heuristics are over-built · Medium
Because of [FORMAT-NOTES.md](FORMAT-NOTES.md) §2, the fingerprint heuristics of
§6.1 may do more harm than good (they can mis-match and silently move history).

**Fix:** make the recorded-map match (CH-A7) the primary path, fingerprinting the
fallback, and have a strict default: when in doubt, treat as delete+add and
**log it loudly** rather than guess. Provide `stem rematch` to correct.

### CH-B4 — Freeze files are excluded, but frozen tracks reference them · High
§8 excludes `Samples/Processed/Freeze`, yet a frozen track in the Set references
that freeze audio. Checkout then yields a Set that believes it is frozen with no
audio behind it, which is exactly the "missing-file prompt" FR1/Goal 1 forbid.
The sample uses Freeze (158 mentions).

**Fix:** detect frozen tracks and either (a) include their freeze file as a
required asset, or (b) on checkout mark the track unfrozen and warn. Never produce
a silently broken frozen track.

### CH-B5 — Sample mtime/CRC means "checkout" may not be invisible to Live · High
Sample refs carry `OriginalFileSize`, `OriginalCrc`, `LastModDate`
([FORMAT-NOTES.md](FORMAT-NOTES.md) §3). A checkout that rewrites bytes or sets a
new mtime may make Live flag samples as changed/replaced.

**Fix:** on materialize, **restore the recorded `LastModDate`** and verify the
CRC; add this to the Phase 0 spike's open question 2/3 (it is really "which
metadata does Live trust?").

### CH-B6 — One-way Live version upgrades have no mechanism · High
Opening a Set in a newer Live and saving silently upgrades it, and older Live
cannot open it (Goal "checkout yields an openable Set" breaks for a teammate on an
older Live). §13 lists this as a risk with no enforcement.

**Fix:** a commit hook that blocks a `Creator`/`MinorVersion`/`SchemaChangeCount`
bump unless `--allow-upgrade` is passed; `stem.toml` pins a minimum and a maximum
supported Live version; `stem doctor` reports the Set's version and the machine's.

### CH-B7 — Max for Live devices are both assets and opaque state · Medium
The sample has 10 `MxDBlob`s. `MxDPatchRef`/`MxDIdRef` reference an `.amxd` patch
that may be embedded or external. §8 only covers VST/AU.

**Fix:** treat `.amxd` as an asset with its own content-addressed entry; treat
`MxDBlob` as an opaque byte blob keyed by SID, same policy as plugin state.

## C. Questionable choices

### CH-C1 — Renaming colliding samples on ingest mutates files under a live Set · High
§8 says on ingest Stem "renames the incoming file with a short hash suffix." That
edits `Samples/` under a Set Live may have open, violating NFR3 and the §10 rule
"never rewrite the open .als underneath a running Live."

**Fix:** resolve name collisions at **merge/implode** time by materializing the
incoming blob under a new name and rewriting the *canonical* reference; never
touch the user's working `Samples/` on ingest.

### CH-C2 — SHA-256 manifest duplicates what Git LFS already content-addresses · Medium
§8 stores assets by SHA-256 *and* puts them in LFS (which also addresses by hash).
Two sources of truth. The sample's `manifest.json`-worth of data (origin, license,
external refs) is valuable; the hash duplication is not.

**Fix:** let LFS own bytes and content addressing; keep `manifest.json` for
**metadata only** (logical path, origin, license, external flag). Keep the
object-store backend behind the same interface for later.

### CH-C3 — "Incremental parse" of a gzip stream is not possible · Medium
NFR2 says commit of a "200 MB-of-XML Set completes in under 5 seconds … after the
first run (incremental parse)." There is no incremental parse of a gzip stream;
every save is a full gunzip+parse. What can be incremental is *emitting and
hashing only the entities that changed*.

**Fix:** restate NFR2 as streaming parse + change-set emission; make it the
language-selection driver (Go or Rust, not Python).

### CH-C4 — Lock refs without compare-and-swap can double-grant · Medium
§10 pushes lock refs (`refs/stem/locks/<sid>`). Two users can both read "unlocked"
and both push. Also, Git LFS locks are per path, but locks are per SID.

**Fix:** update via `git push --force-with-lease=<ref>:<expected>` so a stale
lease fails the second push; for LFS, hold a sentinel path per track SID.

### CH-C5 — "newest" merge policy has no reliable clock · Low
§9 offers a `newest` policy. Commit timestamps are user-settable and rebase-stable.
**Fix:** drop it, or define it as "newest by committer date in the merge inputs,
ties go to `ours`," and note it is advisory.

### CH-C6 — Property tests assume commutativity the algorithm doesn't guarantee · Medium
§12 asserts `merge(a,b) == merge(b,a)`. That only holds with deterministic
tie-breaking. A plain sequence merge of two insertions at the same position is
order-dependent.
**Fix:** specify tie-breaks keyed by SID (and stable ordering of conflict reports),
then the property can hold; keep the test and make it a spec.

## D. Sequencing and process

### CH-D1 — NFR4 "degrade to plain Git" is currently false · High
Once `*.als` is ignored and exploded into a tree, a clone without Stem has **no
`.als`** and no meaningful diffs. "Plain Git behavior" is not a fallback that
exists.
**Fix:** restate NFR4 as "no data loss and a documented recovery path (`stem
implode`, plus a hidden backup ref)"; ship the standalone implode binary (CH-A1).

### CH-D2 — Merge is deferred ~4–5 months; prove feasibility in the spike · High
Merge is the differentiator (§1) but lands in Phase 3 after diff and versioning.
If the canonical model cannot merge, Phases 1–2 are built on sand.
**Fix:** add a **merge feasibility proof** to Phase 0: hand-build two branches that
each add a track, implode both plus a merge, open in Live. Also prove the
return-track/send case (CH-B1).

### CH-D3 — Live smoke tests are real infrastructure, not a footnote · Medium
§12's nightly macOS+Windows UI-automation suite needs licensed machines and
maintenance. On Linux CI the best possible check is explode→implode→explode fixed
point, which is necessary but not sufficient (it cannot prove Live *accepts* it).
**Fix:** budget for a dedicated smoke-test host from the start; treat "Live opens
it" as a first-class test artifact with screenshots.

### CH-D4 — The gunzip fallback should be milestone 1, not a "fallback mode" · Medium
§15 mentions the gunzip-only filter as a stepping stone. It is ~a week of work and
immediately useful (line diffs today). Ship it as M1 to start dogfooding.
**Fix:** make it the first deliverable, behind the same asset/store architecture.

### CH-D5 — Licensing review belongs in Phase 0 · Low
§16 rates ToS/format stability Low. Given an undocumented format and a smoke
suite that drives Live, get legal input before public beta.
**Fix:** move the review into Phase 0; keep adapters thin and avoid redistributing
format specs.

## Summary table

| ID | Title | Severity |
| --- | --- | --- |
| CH-A1 | Filters can't explode one file into many | Blocker |
| CH-A2 | Per-file merge driver can't do a whole-tree merge | Blocker |
| CH-A3 | Paths encode order/name, fighting SIDs + Windows | High |
| CH-A4 | Fixed-precision floats are lossy (breaks NFR1) | High |
| CH-A5 | Blind re-sorting contradicts unknown-passthrough | High |
| CH-A6 | §9 contradicts itself on ID reconciliation | High |
| CH-A7 | SID matching ignores Stem's own Id→SID map | High |
| CH-B1 | Sends must be keyed by return SID | High |
| CH-B2 | Groups, racks, Drum pads not modeled | High |
| CH-B3 | Match heuristics over-built given stable Ids | Medium |
| CH-B4 | Excluded freeze files break frozen tracks | High |
| CH-B5 | mtime/CRC may make checkout visible to Live | High |
| CH-B6 | No guard on one-way version upgrade | High |
| CH-B7 | Max for Live devices/assets not covered | Medium |
| CH-C1 | Ingest renames samples under a live Set | High |
| CH-C2 | Hash manifest duplicates LFS | Medium |
| CH-C3 | "Incremental parse" of gzip is impossible | Medium |
| CH-C4 | Lock refs need compare-and-swap | Medium |
| CH-C5 | "newest" policy lacks a clock | Low |
| CH-C6 | Commutativity needs defined tie-breaks | Medium |
| CH-D1 | "Degrade to plain Git" is currently false | High |
| CH-D2 | Merge feasibility not proven until month ~5 | High |
| CH-D3 | Live smoke suite underestimated | Medium |
| CH-D4 | Gunzip fallback should be M1 | Medium |
| CH-D5 | Licensing review timing | Low |
