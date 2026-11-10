# TDD: Version Control and Collaboration for Ableton Live Projects

Status: Draft v0.2 (revised) · Date: November 10, 2026 · Codename: Stem
Supersedes: [TDD-v0.1.md](TDD-v0.1.md) · Review: [CRITIQUE.md](CRITIQUE.md) · Evidence: [FORMAT-NOTES.md](FORMAT-NOTES.md)

> v0.2 applies the fixes from the critique. Changed sections are marked **[rev]**.
> Each carries the critique IDs (e.g. CH-A1) that motivated it.

## 0. Changelog from v0.1

- **[rev] §5/§9** Git integration reworked: `*.als` is a gitignored build
  artifact; merges are whole-tree operations run by `stem`, not per-file drivers.
  (CH-A1, CH-A2)
- **[rev] §6** Canonical form: SID-based paths, exact-text numerics, no blind
  re-sorting, explicit ID spaces, sends keyed by return SID. (CH-A3, CH-A4, CH-A5,
  CH-B1, CH-B2)
- **[rev] §6.1** Identity: persist the Id→SID map Stem writes; fingerprinting is a
  fallback. (CH-A7, CH-B3)
- **[rev] §8** Assets: LFS owns bytes; manifest is metadata-only; collisions
  resolved at implode time; freeze files handled; Max for Live covered. (CH-B4,
  CH-B7, CH-C1, CH-C2)
- **[rev] §9** Clean SID-only merge with defined tie-breaks and no double
  renumbering. (CH-A6, CH-C6)
- **[rev] §10** Locks use compare-and-swap. (CH-C4)
- **[rev] §12/§14** Merge feasibility moved into Phase 0; live smoke suite
  budgeted; gunzip fallback is M1. (CH-D2, CH-D3, CH-D4)
- **[rev] §13** Open questions updated with sample findings.

## 1. Summary

Stem treats an Ableton Live project folder the way Git treats a source tree. A
Live Set (`.als`) is decomposed into a deterministic, diffable, mergeable
canonical form. Audio assets are stored content-addressed. Contributors work on
branches, review semantic diffs (tracks added, clips moved, device parameters
changed), and merge with entity-level conflict resolution instead of whole-file
conflicts.

Stem is a layer on top of Git, not a replacement. Any Git host works as the
remote. The novel work is: (1) a lossless normalizer between `.als` and a canonical
on-disk form, (2) a semantic diff engine, (3) a whole-tree structural three-way
merge with ID reconciliation, and (4) asset and plugin-state handling that keeps
repositories small and portable.

## 2. Goals and Non-Goals

### Goals
- Every commit is a restorable, openable Live Set: checkout yields a project Live
  opens without missing-file prompts or repairs.
- Diffs are human-readable at the musical level, not the XML level.
- Two people can edit different tracks, scenes, or arrangement regions in parallel
  and merge automatically in the common case.
- Repos stay small: audio is deduplicated, generated files (freezes, analysis
  files, backups) are excluded or regenerable.
- Works offline, with standard Git remotes, on macOS and Windows.

### Non-Goals
- Real-time co-editing inside a running Live session. Live has no extension point
  for this.
- Merging opaque plugin state at the parameter level (see §8).
- Replacing cloud sample libraries or stem-sharing platforms.
- Rendering audio headlessly (Live exposes no CLI renderer).

## 3. Background: Anatomy of a Live Project

Validated against a real Live 11.3.43 Set; see [FORMAT-NOTES.md](FORMAT-NOTES.md).

```
MySong Project/
  MySong.als                  # gzip-compressed XML: the entire Set
  Samples/
    Imported/                 # files dragged in from elsewhere
    Recorded/                 # audio recorded in Live
    Processed/                # Consolidate, Crop, Freeze, Reverse outputs
  Backup/                     # Live's own timestamped autosaves
  Ableton Project Info/       # project metadata
  *.asd                       # per-sample analysis (warp markers, etc.)
```

Measured properties that drive the design:

| Property | Measured fact | Consequence |
| --- | --- | --- |
| Gzip XML | 0.5 MB gzip → 5.1 MB XML | Raw Git sees an opaque blob; must decompress and normalize. |
| Root version attrs | `MajorVersion`, `MinorVersion`, `SchemaChangeCount`, `Creator`, `Revision` | Per-version adapter selected from the root; pin min/max Live version. |
| Many ID spaces | 13,536 `Id`-bearing elements across 140+ element names; `NextPointeeId` is a separate counter | Not one counter. Model each ID space; regenerate on implode. (CH-A6) |
| Ids stable across saves | All pre-existing track Ids identical across main/backup | The Live Id, scoped by type+parent, is a strong identity signal. (CH-A7) |
| Inline opaque state | `MxDBlob` holds hex text (some JSON) inside the XML | Extract to sidecar files, keyed by SID. |
| Sample refs | Carry relative+absolute `Path`, `OriginalFileSize`, `OriginalCrc`, `LastModDate`, `LivePackId` | Must rewrite paths and restore mtime/CRC on checkout. (CH-B5) |
| Automation scaffolding | 6,413 `AutomationTarget`, 4,142 `ModulationTarget`, 11,466 `LockEnvelope` | Most of the file is per-parameter boilerplate; canonical form must regenerate it deterministically. |
| Ordering stability | Not yet measured (needs controlled double-save) | Do **not** reorder unconditionally. (CH-A5) |

## 4. Requirements

### Functional
- FR1: Initialize a repo from an existing project folder without moving files Live
  depends on.
- FR2: Commit, branch, merge, rebase, tag, revert, and cherry-pick at the Set level.
- FR3: Produce a semantic diff between any two commits.
- FR4: Merge two Set versions with automatic resolution of non-overlapping changes.
- FR5: Detect and report missing plugins, missing samples, frozen tracks with
  missing freeze audio, and Live version mismatches on checkout.
- FR6: Support pessimistic locks on tracks or on the whole Set for contested
  resources.
- FR7: Attach optional reference bounces (MP3 or WAV) to commits for audible review.
- FR8: Provide history of a single entity (this track, this clip, this device).

### Non-Functional
- NFR1: **Round-trip fidelity.** After explode then implode, Live must open the Set,
  and explode of the re-saved Set must produce an identical canonical tree. This is
  a fidelity requirement, not a byte requirement: canonical *equality* is defined on
  the canonical tree, with raw text preserved for lossless implode. (CH-A4)
- NFR2 **[rev]**: Commit of a 200 MB-of-XML Set completes in under 5 seconds on a
  mid-range laptop after the first run. There is no incremental gzip parse; the
  incremental part is emitting and hashing only changed entities. (CH-C3)
- NFR3: Never mutate the user's working Set or its `Samples/` without a recoverable
  backup (a hidden ref plus Live's own `Backup/` folder). (CH-C1)
- NFR4 **[rev]**: No Stem failure may cause data loss, and recovery must be
  documented and available without Stem: a static `stem implode` binary plus a
  hidden backup ref. "Degrade to plain Git" is **not** claimed, because a
  Stem-managed repo has no committed `.als`. (CH-D1)

## 5. Architecture

```
  Live (user DAW)
      |  saves MySong.als        (build artifact, gitignored)
      v
  Watcher / CLI  --->  Normalizer (gunzip, parse, entity graph, canonicalize)
                           |
                           v
                  Exploded canonical tree  --->  Git (objects, refs)
                           |                         |
                           |                         +--> Remote (any Git host)
                           v
                  Asset store (LFS) -> optional object storage

  Diff engine   <--  two canonical trees
  Merge engine  <--  base, ours, theirs canonical trees   (whole-tree, in-process)
  Implode       -->  rebuilt .als for Live to open
```

### Git integration **[rev]** (CH-A1, CH-A2, CH-D1)
- The working `.als` is a **gitignored build artifact**. It is never tracked.
- `stem commit` explodes the Set into the canonical tree and commits that tree.
- `stem checkout` / `switch` / `clone` implode the canonical tree into a working
  `.als` (respecting §10 when Live is open).
- `merge`, `rebase`, and `cherry-pick` are **whole-tree operations performed by
  `stem`** over the three commit trees, not Git drivers. Git's own text merge is
  never used for `set/**`.
- A `merge=stem` attribute exists only as a safety net: if stock Git is used
  anyway, the driver emits a conservative conflict and never guesses.

### Components
1. **Normalizer.** Parses `.als` into an entity graph; version-aware adapter chosen
   from root attributes. Emits canonical form.
2. **Canonical store.** On-disk repository form (§6).
3. **Implode.** Rebuilds a valid `.als`: regenerates all ID spaces and counters,
   rewrites asset paths and metadata for the local machine, preserves unknown XML.
4. **Diff engine.** Entity-aware comparison producing a typed change list (§7).
5. **Merge engine.** Library over three in-memory trees; structural merge with SID
   identity (§9). Called by the CLI, not by Git.
6. **Asset manager.** Content addressing via LFS, plus a metadata manifest (§8).
7. **Watcher.** Optional; snapshots saves to a private ref.
8. **Review UI.** Local web viewer showing arrangement timeline diffs and playing
   reference bounces.

### Why a layer on Git
Git provides content-addressed storage, DAG history, remotes, permissions, and
review ecosystems. Stem integrates via hooks, `textconv`, and its own merge
command. It uses clean/smudge **only** for line-oriented sidecars (e.g. `.state`
blobs), never to synthesize a tree.

## 6. Canonical Representation **[rev]**

The repo does not store the `.als`. It stores an exploded, canonicalized form.
The `.als` in the working tree is a build artifact (gitignored) rebuilt on
checkout or by `stem implode`.

```
repo/
  stem.toml                       # project config, Live version pins, plugin manifest
  set/
    set.json                      # tempo map, time signature map, scale, globals
    order.json                    # explicit ordering of tracks/returns/scenes (by SID)
    master/
      track.xml
      devices/<sid>.xml
    returns/
      order.json
      <return-sid>/
        track.xml
        devices/<sid>.xml
    tracks/
      <sid>/
        track.xml                 # mixer, routing, color, name, parent SID, group SID
        devices/<sid>.xml
        devices/<sid>.state       # opaque blob (VST/AU/Max), byte-exact
        clips/session/<sid>.xml
        clips/arrangement/<sid>.xml
        automation/<sid>.json
    scenes/<sid>.json
    locators.json
  assets/
    manifest.json                 # metadata only: logical path, origin, license, external flag
    objects/                       # LFS pointers (or LFS smudge targets)
  identity.json                   # committed: Live Id <-> SID map (see 6.1)
  .stem/                          # local state, ignored
```

### Canonicalization rules **[rev]**
- **Paths are SID-based.** Human names and order live in `order.json` and entity
  documents, never in paths. This makes rename/reorder cheap and Windows-safe.
  (CH-A3)
- **Numbers keep their exact source text.** No precision normalization in the
  canonical bytes; equality is computed on a normalized projection, implode uses
  the raw text. (CH-A4)
- **Ordering is preserved, not invented.** Children keep source order unless the
  collection is *known* unordered; only those are sorted by (order index, SID).
  (CH-A5)
- **Stable entity IDs (SIDs).** A UUID per entity, stored in the canonical form and
  mapped to Live's numeric Ids only at implode.
- **Explicit ID spaces.** The model tracks, per entity, which ID space and which
  Id it owned at the last implode: per-list `Id`, `PoineeId` ranges
  (`AutomationTarget`, `ModulationTarget`, `Pointee`), `LomId`, track-routing Ids,
  `TrackSendHolder`, `ClipSlot`, `Scene`, `WarpMarker`, `FileRef`/`SourceContext`,
  `MxD*`. All are regenerated deterministically at implode. (CH-A6)
- **Sends are a map keyed by return SID**, not a positional list. (CH-B1)
- **Track hierarchy is explicit** (`parent`/`group` SID) so groups and racks merge
  as trees. (CH-B2)
- **Opaque blobs are separate, byte-exact files** keyed by owning SID, so they never
  pollute text diffs.
- **Volatile view state** (zoom, scroll, selection, window geometry) is split into a
  non-merged sidecar or dropped, per config.

### 6.1 Entity identity across saves **[rev]** (CH-A7, CH-B3)
Stem controls implode, so it knows the exact Live Id→SID assignment it produced.
Ingest, in priority order:
1. **Recorded map.** Look up the Live Id (scoped by type + parent SID) in
   `identity.json` / `.stem`. This is exact for entities that existed at the last
   implode. (Evidence: Live kept Ids stable across saves —
   [FORMAT-NOTES.md](FORMAT-NOTES.md) §2.)
2. **Structural fingerprint** within the same parent (name, device-chain signature,
   clip content hash), for entities Live created since.
3. **Unmatched → new SID; disappeared → deletion.**
4. **Ambiguity resolves conservatively** (delete + add) and is written to a
   user-visible match log. `stem rematch` lets a user correct a mapping.

## 7. Semantic Diff

Unchanged in substance from v0.1 §7. The engine compares two canonical trees and
emits a typed, SID-keyed change list (tracks, clips, MIDI notes, devices,
parameters, automation, plugin blobs, samples, tempo).

Additions **[rev]**:
- Return/send changes render as `~ send Vox -> A-drum-verb: -3.0 -> -6.0 dB`.
- Group membership changes render as `~ track 'kick' moved into group 'BUS_KICK'`.
- Frozen-track state changes are surfaced because they affect assets.
- `textconv` filters render canonical XML/JSON as readable text for stock
  `git diff`; the semantic change list is the primary output.

Output formats unchanged: terminal, JSON, Review UI timeline.

## 8. Assets and Plugin State **[rev]**

### Audio assets
- **Bytes are owned by Git LFS** (content-addressed by the LFS pointer). The
  `manifest.json` stores **metadata only**: logical path, origin, license, external
  flag. No SHA-256 duplication of LFS. (CH-C2)
- On checkout, assets are materialized into `Samples/` at their original relative
  layout. The recorded **`LastModDate` is restored** and the `OriginalCrc` verified
  so Live does not treat samples as changed. (CH-B5)
- **Name collisions are resolved at implode/merge time**, by materializing the
  incoming blob under a hash-suffixed name and rewriting the *canonical* reference.
  The user's working `Samples/` is never renamed during ingest. (CH-C1)
- **Freeze files.** A frozen track references its freeze audio. Such files are
  **required assets** of the frozen track; if excluded, checkout must unfreeze the
  track and warn, never leave a frozen track with no audio. (CH-B4)
- **`.asd` analysis** is regenerable and excluded by default; `stem doctor` reports
  whether inline warp/analysis data is sufficient (open question 3).
- **External references** (Core Library, Live Packs, sample libraries) are recorded
  in the manifest with `LivePackName`/`LivePackId`; `stem doctor` lists unresolved
  ones.
- Recommend Live's **Collect All and Save** before first commit.

### Plugin and Max for Live state
- Each opaque blob (VST2/VST3/AU state, Max `.amxd` embedded state, `MxDBlob`) is a
  byte-exact sidecar keyed by the owning device SID. (CH-B7)
- Atomic in merges: one side changed → take it; both changed → conflict.
- `stem.toml` records name, vendor, format, and version seen at commit; `stem
  doctor` reports missing/mismatched plugins on checkout.
- Native Ableton devices and racks are parsed into parameters and are diffable and
  mergeable.
- Team guidance: save important sounds as presets or native racks.

## 9. Merge Design **[rev]**

### Where it runs (CH-A2)
The merge engine operates on three **in-memory trees** (base, ours, theirs) loaded
by `stem merge` / `rebase` / `cherry-pick`. It writes the merged tree and lets Stem
create the commit. Git's per-file merge is not used for the canonical tree.

### Algorithm
1. Match entities across all three trees by SID (identity from §6.1 for the Set
   being merged; SIDs in the trees are authoritative).
2. Classify each entity: unchanged, one-side change, both-side change, add (one/both),
   delete (one/both).
3. Merge property-by-property for changed entities; disjoint properties merge
   cleanly.
4. Merge ordered collections (track order, device chains, arrangement clips, sends
   as a keyed map) with a sequence merge **keyed by SID**. Ties (two insertions at
   the same position) are broken by a **deterministic SID ordering**, so
   `merge(a,b) == merge(b,a)`. (CH-C6)
5. **No merge-time Live Ids.** SIDs are the only merge-time identity; all Live Ids
   and counters are assigned by implode. There is no "renumber one side" step.
   (CH-A6)
6. Validate the merged tree against referential invariants (no dangling references,
   routing targets exist, no duplicate SIDs). Failure aborts into a conflict state.

### Conflict classification (default policy)

| Situation | Policy |
| --- | --- |
| Different tracks edited | Auto-merge |
| Same track, different devices or different clips | Auto-merge |
| Same clip, non-overlapping bar ranges | Auto-merge notes; flag if note sets overlap in time and pitch |
| Two branches add different return tracks | Auto-merge (sends keyed by return SID) |
| Same scalar parameter, different values | Conflict; interactive or configured `ours`/`theirs` |
| Track deleted on one side, edited on the other | Conflict; default keep and show edits |
| Same plugin blob changed on both sides | Conflict; pick one, or keep both as instances (opt-in) |
| Tempo, time signature, or key changed on both sides | Conflict, always interactive |
| Both sides add a track with the same name | Not a conflict; both kept (distinct SIDs) |
| Sample replaced on both sides with different hashes | Conflict; keep both assets, pick reference |
| Frozen/unfrozen vs freeze-file edit | Conflict; surface because it affects assets |

### Resolution UX
Unchanged from v0.1 §9 (semantic conflict entries, non-interactive flags for CI).

## 10. Collaboration Model **[rev]**

### Branching
Unchanged: feature branches by role, section ownership, trunk-with-locks.

### Locking (CH-C4)
- Granularity: whole Set, track, or scene.
- Transport: Git LFS locking where supported; otherwise lock refs
  `refs/stem/locks/<sid>`.
- **All lock acquisition and release use compare-and-swap**:
  `git push --force-with-lease=<ref>:<expected>` so a stale lease fails rather than
  double-granting. For LFS, hold a sentinel path per track SID.
- Advisory by default; enforced by a pre-push hook in strict mode.
- Locks expire and can be force-released by admins.

### Review
Unchanged: PR comment with semantic summary; optional reference bounce in LFS;
comments anchored to a bar range or track.

### Working with Live open
Unchanged rules: detect Live holding the file; never rewrite the open `.als`;
write a sibling `MySong (stem checkout).als` or ask the user to close Live;
watcher autosaves go to `refs/stem/autosave/<user>` (never pushed).

## 11. CLI and Configuration

```
stem init                       # adopt the current project folder
stem implode [set.als]          # standalone: canonical tree -> .als (no deps)
stem status                     # dirty state, locks, missing assets/plugins/freeze
stem commit -m 'tighten drums'  # normalize, explode, git commit
stem diff [a] [b]               # semantic diff
stem log --entity track:Bass    # history of one entity
stem branch / checkout / switch # implode on checkout
stem merge <branch>             # whole-tree structural merge
stem rebase / cherry-pick       # whole-tree
stem lock track:Vocals          # advisory or enforced (CAS) lock
stem rematch                    # correct an identity mapping
stem doctor                     # paths, plugins, Live version, freeze audit
stem bounce attach mix.mp3      # attach reference audio
```

Example `stem.toml` (additions marked):

```toml
[project]
min_live_version = '11.0'
max_live_version = '12.x'        # [rev] block accidental upgrades
allow_live_upgrade = false        # [rev] commit hook guard

[canonical]
drop_view_state = true
split_automation = true
preserve_source_order = true      # [rev] do not re-sort blind

[assets]
store = 'lfs'
exclude = ['**/*.asd', 'Backup']
require_freeze_files = true       # [rev]
restore_mtime = true              # [rev]

[plugins.required]
Serum = { format = 'VST3', min_version = '1.3' }

[merge]
scalar_conflict = 'interactive'
plugin_blob_conflict = 'interactive'

[locks]
mode = 'advisory'
expiry_hours = 24
compare_and_swap = true           # [rev]
```

`stem init` writes `.gitignore` (for `*.als`, `Backup/`, `*.asd`) and
`.gitattributes` **[rev]**:

```gitattributes
set/**  diff=stem text
*/*.state diff=stem-blob -text
assets/** filter=lfs diff=lfs merge=lfs -text
# no merge=stem driver on set/** : merges are done by `stem merge` (§9)
```

## 12. Testing Strategy **[rev]**

- **Corpus.** Real projects across small/medium/huge and session/arrangement/rack/
  plugin-heavy, per supported Live version, with owner permission; plus synthetic
  Sets. Current corpus is one Live 11.3.43 set (see [FORMAT-NOTES.md](FORMAT-NOTES.md)).
- **Round-trip.** explode → implode → open → re-save → explode must be a fixed
  point on the canonical tree. Byte-identity is not required (NFR1); canonical
  equality is.
- **Ordering stability (new, first).** Save the same Set twice with no edits;
  compare decompressed XML byte-for-byte. This decides CH-A5.
- **Live smoke tests.** Nightly macOS + Windows UI automation that opens imploded
  Sets and re-saves; artifacts include screenshots. This is real infrastructure
  (dedicated host), budgeted in Phase 0. (CH-D3)
- **Identity tests.** Mutation harness (rename, move, duplicate, reorder, delete,
  add) through a simulated Live save; assert the SID mapping. Includes the
  recorded-map path and the fingerprint fallback.
- **Merge property tests.** Random edit pairs on disjoint entities: assert
  `merge(a,b) == merge(b,a)` (with defined SID tie-breaks) and idempotence;
  overlapping edits must raise a conflict, never silently lose data.
- **Schema validation.** Every merged tree satisfies referential invariants.
- **Fuzzing.** Malformed/truncated gzip and XML produce clear errors, never corrupt
  the repo.
- **Performance.** Commit/diff/implode times tracked on the largest corpus Sets.

## 13. Risks and Open Questions

| Risk | Likelihood | Impact | Mitigation |
| --- | --- | --- | --- |
| .als schema changes between releases | High | High | Per-version adapters, unknown passthrough, corpus regression per release |
| Implode produces a Set Live rejects or alters | Medium | Critical | Preserve unknown XML verbatim, raw numeric text, round-trip + smoke tests, hidden backup ref |
| One-way version upgrade on save | High | High | Version pins + commit-hook guard (CH-B6) |
| SID matching mis-identifies entities | Low→Medium | High | Recorded Id→SID map primary; conservative fallback; match log; `stem rematch` |
| Plugin/blob conflicts frustrate users | High | Medium | Locks, rack guidance, keep-both |
| Frozen track with missing freeze audio | Medium | High | Require freeze files or unfreeze+warn (CH-B4) |
| Checkout makes Live think samples changed | Medium | Medium | Restore mtime, verify CRC (CH-B5) |
| Large audio repos overwhelm hosts | Medium | Medium | Exclusions, LFS/object store, shallow/partial clone |
| Live edits while Stem rewrites | Medium | High | Live-open detection, sibling-file checkout, never silent overwrite |
| Legal/format-stability | Low | Medium | License review in Phase 0; thin adapters; no spec redistribution |

### Open questions (updated)
1. **Id stability.** Sample evidence: stable for existing entities across two saves.
   Needs confirmation over the full corpus and across a Live restart / collect-all.
2. **Volatile fields.** Which can be dropped without Live re-deriving something
   visible? (zoom, selection, scroll, `LastModDate` trust.)
3. **Warp/analysis authority.** Both inline data and `.asd` sidecars exist. Which is
   authoritative, and does `OriginalCrc`/`LastModDate` need to match for Live to
   skip re-analysis?
4. **Locking hosts.** Which Git hosts support LFS locking well enough, and does the
   CAS lock-ref scheme work on them?
5. **Live Packs / templates.** `.alp` and template structure for sharing instruments.
6. **NEW — ordering stability.** Does Live produce byte-stable XML for a no-edit
   re-save? (Highest-priority Phase 0 experiment.)

## 14. Milestones **[rev]**

| Phase | Scope | Exit criteria |
| --- | --- | --- |
| 0. Spike (3–4 wk) | Parse+canonicalize one Live version; **measure ordering stability**; answer Q1–3,6; **prove merge feasibility** (two branches each add a track + a return, merge, open in Live); prove explode-implode-open on real projects | Round-trip fixed point; Live opens all outputs; a merged Set opens with both branches' edits |
| 1. Versioning (6–8 wk) | `init`, `commit`, `checkout`, **standalone `implode`**, asset store (LFS), `doctor`, **gunzip textconv fallback (M1)** | Daily use on real songs; no data loss in 4 wk dogfooding |
| 2. Semantic diff (4–6 wk) | Typed change list, `log --entity`, JSON, minimal Review UI timeline | Diffs on corpus judged readable by 5 producers |
| 3. Merge (8–10 wk) | Whole-tree merge engine, identity hardening, conflict UX, property tests | Two-person trial: 20 merges, zero broken Sets |
| 4. Collaboration (6 wk) | Locks (CAS), ownership, PR integration, reference bounces, Review UI audio compare | Team of 4 completes an EP on Stem |
| 5. Hardening | Multi-version support, Windows parity, perf, docs, optional Max for Live status device | Public beta |

## 15. Alternatives Considered

Unchanged from v0.1 §15, with one correction: the **gunzip-only filter** is not
merely a fallback — it is shipped as M1 (§14) behind the same asset/store
architecture. (CH-D4)

## 16. Security, Privacy, and Licensing

Unchanged from v0.1 §16, with the licensing review moved into Phase 0. (CH-D5)

## 17. Glossary

- Set: a Live document (`.als`).
- Canonical form: Stem's normalized, exploded representation of a Set stored in Git.
- Explode / Implode: convert a Set to canonical form and back.
- SID: stable entity ID assigned by Stem, independent of Live's numeric Ids.
- Opaque blob: binary plugin/Max state that Stem stores byte-exact and does not
  interpret.
- ID space: one of Live's independent numeric-Id namespaces (`Id`, pointees,
  `LomId`, clip slots, warp markers, …).
