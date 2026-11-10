# TDD: Version Control and Collaboration for Ableton Live Projects

Status: Draft v0.1 · Date: November 10, 2026 · Codename: Stem

> This is the original draft, preserved verbatim as the baseline for review.
> The engineering critique lives in [CRITIQUE.md](CRITIQUE.md); the revised
> design is [TDD-v0.2.md](TDD-v0.2.md).

## 1. Summary

Stem treats an Ableton Live project folder the way Git treats a source tree. A Live Set (.als) is decomposed into a deterministic, diffable, mergeable representation. Audio assets are stored content-addressed. Contributors work on branches, review semantic diffs (tracks added, clips moved, device parameters changed), and merge with entity-level conflict resolution instead of whole-file conflicts.

Stem is a layer on top of Git, not a replacement. Any Git host works as the remote. The novel work is: (1) a lossless normalizer between .als and a canonical on-disk form, (2) a semantic diff engine, (3) a structural three-way merge driver with ID reconciliation, and (4) asset and plugin-state handling that keeps repositories small and portable.

## 2. Goals and Non-Goals

### Goals
- Every commit is a restorable, openable Live Set: checkout yields a project Live opens without missing-file prompts or repairs.
- Diffs are human-readable at the musical level, not the XML level.
- Two people can edit different tracks, scenes, or arrangement regions in parallel and merge automatically in the common case.
- Repos stay small: audio is deduplicated, generated files (freezes, analysis files, backups) are excluded or regenerable.
- Works offline, with standard Git remotes, on macOS and Windows.

### Non-Goals
- Real-time co-editing inside a running Live session (Google Docs style). Live has no extension point for this.
- Merging opaque plugin state at the parameter level (see section 8).
- Replacing cloud sample libraries or stem-sharing platforms.
- Rendering audio headlessly (Live exposes no CLI renderer).

## 3. Background: Anatomy of a Live Project

The following is the working model of a project and must be validated by the spike in section 13, since the format is undocumented and changes between Live versions.

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

Key properties of the .als file that drive the design:

| Property | Consequence |
| --- | --- |
| Gzip-compressed XML | Raw Git sees an opaque blob; every save is a full-file change. Must decompress and normalize. |
| Numeric Id attributes on tracks, devices, clips, automation targets, pointees | IDs are allocated per Set from counters. Two branches adding objects will collide. Merge must remap IDs. |
| Large inline plugin state (binary buffers as hex text) | Huge, unreadable, atomic. Diff and merge at blob granularity only. |
| Sample references by relative and absolute path plus metadata | Paths break across machines. Must be rewritten on checkout. |
| Element ordering and whitespace not guaranteed stable across saves | Normalizer must canonicalize ordering and formatting or diffs will be noisy. |
| Version attributes on root element | Sets saved by a newer Live may not open in an older one. Repo must pin a minimum Live version. |

## 4. Requirements

### Functional
- FR1: Initialize a repo from an existing project folder without moving files Live depends on.
- FR2: Commit, branch, merge, rebase, tag, revert, and cherry-pick at the Set level.
- FR3: Produce a semantic diff between any two commits.
- FR4: Merge two Set versions with automatic resolution of non-overlapping changes.
- FR5: Detect and report missing plugins, missing samples, and Live version mismatches on checkout.
- FR6: Support pessimistic locks on tracks or on the whole Set for contested resources.
- FR7: Attach optional reference bounces (MP3 or WAV) to commits for audible review.
- FR8: Provide history of a single entity (this track, this clip, this device).

### Non-Functional
- NFR1: Round-trip safety. After explode then implode, Live must open the Set and a subsequent re-save must normalize to an identical canonical form.
- NFR2: Commit of a 200 MB-of-XML Set completes in under 5 seconds on a mid-range laptop after the first run (incremental parse).
- NFR3: Never mutate the user's working Set without a recoverable backup (a hidden ref plus Live's own Backup folder).
- NFR4: Failure of any Stem component must degrade to plain Git behavior, never to data loss.

## 5. Architecture

```
  Live (user DAW)
      |  saves MySong.als
      v
  Watcher / CLI  --->  Normalizer (gunzip, parse, canonicalize)
                           |
                           v
                  Exploded canonical tree  --->  Git (objects, refs)
                           |                         |
                           |                         +--> Remote (any Git host)
                           v
                  Asset store (hash-addressed) --> LFS or object storage

  Diff engine  <--  two canonical trees
  Merge driver <--  base, ours, theirs canonical trees
  Implode      -->  rebuilt .als for Live to open
```

### Components
1. Normalizer. Parses the .als XML into an in-memory entity graph and emits the canonical form. Version-aware: one adapter per supported Live major version, selected by the root element's version attributes.
2. Canonical store. The on-disk repository form (section 6).
3. Implode. Rebuilds a valid .als from the canonical tree, regenerates the counters Live expects, rewrites asset paths for the local machine.
4. Diff engine. Entity-aware comparison producing a typed change list (section 7).
5. Merge driver. Registered as a Git merge driver for the canonical tree; performs structural three-way merge with ID reconciliation (section 9).
6. Asset manager. Hashes audio, deduplicates, maintains the manifest, and handles transfer through Git LFS or a compatible object store (section 8).
7. Watcher. Optional background process that detects saves of the .als and offers auto-snapshots to a private ref.
8. Review UI. Local web viewer (and later a hosted one) showing arrangement timeline diffs and playing attached reference bounces.

### Why a layer on Git
Git already provides content-addressed storage, DAG history, remotes, permissions, and code-review ecosystems. Reimplementing these adds risk and no musical value. Stem integrates through the standard extension points: clean and smudge filters, textconv, custom merge drivers, and hooks.

## 6. Canonical Representation

The central design decision: the repo does not store the .als. It stores an exploded, canonicalized form of it. The .als in the working tree is a build artifact that can be regenerated (implode) and is rebuilt on checkout.

```
repo/
  stem.toml                       # project config, min Live version, plugin manifest
  set/
    set.json                      # tempo, time signature, scale, global settings
    master.xml                    # master track and its devices
    returns/
      A-reverb.xml
    tracks/
      0007-drums/
        track.xml                 # mixer, routing, color, name, device chain skeleton
        devices/
          01-drum-rack.xml
          02-eq-eight.xml
          03-vst-serum.state      # opaque plugin blob, stored raw
        clips/
          session/slot-03.xml
          arrangement/clip-0042.xml
        automation/
          0042-mixer-volume.json
    scenes/
      scene-0001.json
    locators.json
  assets/
    manifest.json                 # path -> sha256, size, origin
  .stem/                          # local state, ignored
```

### Canonicalization rules
- Stable ordering: children sorted by a stable key (track order index, then stable ID), not by incidental XML order.
- Normalized numerics: fixed precision for floats, no trailing zeros, consistent boolean encoding.
- Stable entity IDs: Stem assigns each entity a stable ID (SID), a UUID stored in the canonical form and mapped to Live's numeric Id at implode time. SIDs persist across saves by matching entities on structure and position, with a fallback to heuristics (section 6.1). SIDs are what make move detection, per-entity history, and merging possible.
- Opaque blobs stored separately, byte-exact, so they never pollute text diffs.
- Volatile fields (view state, window positions, last-opened timestamps) are split into a separate non-merged file, or dropped, according to configuration.

### 6.1 Entity identity across saves
Live does not give us persistent identity; its numeric Ids can be reassigned. When Stem ingests a new save, it matches entities to the previous canonical tree:
1. Exact match on (Live Id, type, parent SID) where stable.
2. Otherwise, match by structural fingerprint (name, device chain signature, clip content hash) within the same parent.
3. Unmatched entities become new SIDs; entities that disappeared become deletions.

Ambiguous matches are resolved conservatively (treat as delete plus add) and logged. This is the highest-risk algorithm in the system and is covered in section 12.

## 7. Semantic Diff

The diff engine compares two canonical trees and emits typed changes keyed by SID:

| Change type | Example output |
| --- | --- |
| Track added or removed | + track 'Lead Synth' (MIDI) |
| Track reordered or renamed | ~ track 'Bass' moved 3 -> 5 |
| Clip added, removed, moved, resized | ~ clip 'Verse Riff' (Guitar) bar 17 -> bar 25 |
| MIDI notes changed | ~ clip 'Pad Chords': +4 notes, -1 note, 2 velocity edits |
| Device added, removed, reordered | + device 'Compressor' after 'EQ Eight' on 'Vocals' |
| Parameter changed (native device) | ~ EQ Eight.Band3.Gain: -2.0 dB -> -4.5 dB |
| Automation changed | ~ automation 'Vocals.Volume': 6 breakpoints edited, bars 33-41 |
| Plugin state changed (opaque) | ~ VST 'Serum' state changed (blob 4.1 KB -> 4.3 KB) |
| Sample replaced | ~ clip 'Kick': sample kick_v1.wav -> kick_v2.wav |
| Tempo or time signature | ~ tempo 120 -> 124 BPM at bar 1 |

Output formats: terminal text, JSON (for tooling), and the Review UI timeline, which renders added, removed, and modified clips as colored regions on an arrangement view.

Integration: git diff uses a textconv filter over the canonical files, so even stock Git tooling shows readable output.

## 8. Assets and Plugin State

### Audio assets
- Stored by SHA-256. The manifest maps logical project-relative paths to hashes.
- Transport: Git LFS by default (works with most hosts); optional S3-compatible object storage for large teams.
- On checkout, assets are materialized into Samples/ using the original relative layout so Live finds them.
- Name collisions: two contributors can each record Audio 1.wav into Samples/Recorded/. On ingest, Stem detects same-path-different-hash and renames the incoming file with a short hash suffix, updating all references in the exploded tree.
- Generated files (freeze files, .asd analysis, Backup/) are excluded by default. Freeze files can be re-generated by the user; .asd files are cheap to rebuild. Configurable.
- Absolute paths: Stem rewrites references to project-relative form where Live permits, and records external references (a sample library elsewhere) in the manifest with a warning. A stem doctor command lists unresolved external references.
- Collect and Save: Stem recommends (and optionally prompts) running Live's Collect All and Save before the first commit so the project is self-contained.

### Plugin state
Third-party plugin state is an opaque binary blob owned by the plugin vendor. Stem cannot meaningfully diff or merge it.
- Stored as its own file per device instance, byte-exact.
- Treated as an atomic value in merges: if only one side changed it, take that side; if both changed it, raise a conflict (section 9).
- The stem.toml manifest records plugin name, vendor, format (VST2, VST3, AU), and version seen at commit time. On checkout, Stem reports plugins that are missing or at a different version.
- Recommended team practice: save important sounds as presets or as Live racks, which are native and diffable.
- Native Ableton devices (EQ Eight, Compressor, Operator, racks) are parsed into parameters and are fully diffable and mergeable.

## 9. Merge Design

### Algorithm
Stem registers as a Git merge driver for the canonical tree. For ours, theirs, and base:
1. Match entities across all three trees by SID.
2. Classify each entity: unchanged, changed on one side, changed on both, added on one or both sides, deleted on one side.
3. Merge property-by-property for changed entities. Changes to different properties of the same entity merge cleanly.
4. Merge ordered collections (track order, device chains, arrangement clips) with an operational-transform-style sequence merge keyed by SID, not by index.
5. Reconcile numeric IDs: when both sides added entities that received the same Live Id, allocate fresh IDs for one side and rewrite all references (pointees, automation targets, routing). Update the Set's next-ID counters. In the SID model this is automatic: SIDs never collide, and Live Ids are assigned at implode time.
6. Validate the merged tree against schema invariants (no dangling references, no duplicate IDs, routing targets exist). Failure aborts the merge into a conflict state rather than producing a broken Set.

### Conflict classification and default policy

| Situation | Policy |
| --- | --- |
| Different tracks edited | Auto-merge |
| Same track, different devices or different clips | Auto-merge |
| Same clip, non-overlapping bar ranges (MIDI notes) | Auto-merge notes; flag if note sets overlap in time and pitch |
| Same scalar parameter changed on both sides to different values | Conflict; interactive pick, or configured policy (ours, theirs, newest) |
| Track deleted on one side, edited on the other | Conflict; default keep and show edits |
| Same plugin blob changed on both sides | Conflict; pick one, or keep both as separate device instances (opt-in) |
| Tempo, time signature, or key changed on both sides | Conflict, always interactive |
| Both sides add a track with the same name | Not a conflict; both kept (distinct SIDs) |
| Sample file replaced on both sides with different hashes | Conflict; keep both assets, pick reference |

### Resolution UX
stem merge prints conflicts as semantic entries, not marker-filled XML:

```
CONFLICT  track 'Vocals' > device 'Compressor' > Threshold
  base:   -18.0 dB
  ours:   -22.0 dB   (alice, commit a1f3c)
  theirs: -15.5 dB   (bo, commit 7d20e)
  resolve: [o]urs  [t]heirs  [e]nter value  [s]kip
```

The same operations are available as non-interactive flags for CI and scripting.

## 10. Collaboration Model

### Branching
Recommended workflows, selectable via stem.toml:
- Feature branches by role. One branch per contributor and task: bo/drums-v2, alice/vocal-comp. Merge to main through review.
- Section ownership. Contributors own tracks or arrangement regions. Stem warns when a commit touches entities owned by someone else (soft ownership via a CODEOWNERS-style file).
- Trunk with locks. For small teams that dislike merging: one active editor per track, enforced by locks.

### Locking
Pessimistic locks complement merging for the things that cannot merge (plugin blobs, tempo maps, big arrangement rewrites).
- Lock granularity: whole Set, a track, or a scene.
- Implemented over the Git LFS locking API where the host supports it; otherwise through a lightweight lock ref (refs/stem/locks/<sid>) pushed to the remote.
- Locks are advisory by default and enforced by a pre-push hook in strict mode.
- Locks expire after a configurable timeout and can be force-released by admins.

### Review
- Pull requests include the semantic diff summary as a generated comment.
- Optional reference bounce: stem bounce attach <file> stores an MP3 alongside the commit (in LFS, as a note). The Review UI plays before and after bounces side by side.
- Comments can anchor to a bar range or a track, not just to a file line.

### Working with Live open
Live does not expose a save hook, and it keeps the Set loaded in memory. Stem therefore follows these rules:
- Never rewrite the open .als underneath a running Live without confirmation. The CLI detects Live holding the file (process and lock detection) and instead writes the checked-out Set to a sibling file (MySong (stem checkout).als) for the user to open, or asks the user to close Live.
- The Watcher snapshots on save into a private ref (refs/stem/autosave/<user>), which never pushes, giving cheap local undo without polluting history.
- A Max for Live companion device is a possible later addition for surfacing status (branch, dirty state, lock owner) inside Live. It is explicitly not on the critical path.

## 11. CLI and Configuration

```
stem init                       # adopt the current project folder
stem status                     # dirty state, locks, missing assets/plugins
stem commit -m 'tighten drums'  # normalize, explode, git commit
stem diff [a] [b]               # semantic diff
stem log --entity track:Bass    # history of one entity
stem branch / checkout / switch # implode on checkout
stem merge <branch>             # structural merge
stem lock track:Vocals          # advisory or enforced lock
stem doctor                     # paths, plugins, Live version audit
stem bounce attach mix.mp3      # attach reference audio
```

Example stem.toml:

```toml
[project]
min_live_version = '12.0'

[canonical]
drop_view_state = true
split_automation = true

[assets]
store = 'lfs'
exclude = ['Samples/Processed/Freeze', '**/*.asd', 'Backup']

[plugins.required]
Serum = { format = 'VST3', min_version = '1.3' }

[locks]
mode = 'advisory'
expiry_hours = 24

[merge.policy]
scalar_conflict = 'interactive'
plugin_blob_conflict = 'interactive'
```

Git integration (written by stem init):

```gitattributes
set/**/*.xml   diff=stem merge=stem
set/**/*.state diff=stem-blob merge=stem
assets/**/*.wav filter=lfs diff=lfs merge=lfs -text
```

## 12. Testing Strategy

- Corpus: a curated set of real projects (small, medium, huge; session-heavy, arrangement-heavy, rack-heavy, third-party-plugin-heavy) saved by each supported Live version, with owner permission, plus synthetic generated Sets.
- Round-trip tests: explode then implode then explode must be a fixed point. Implode output is compared semantically to the original Set.
- Live smoke tests: a scripted check on macOS and Windows that opens imploded Sets and re-saves them, then diffs the canonical output against the input. This is UI-automation based and runs nightly, not per commit.
- Identity tests (SID matching): a mutation harness applies known edits (rename, move, duplicate, reorder, delete) to a Set, re-saves through simulated Live behavior, and asserts the SID mapping.
- Merge property tests: generate random edit pairs on disjoint entities and assert commutativity (merge(a,b) == merge(b,a)) and idempotence; generate overlapping edits and assert a conflict is raised rather than silent data loss.
- Schema validation: every merged tree must satisfy referential invariants; violations fail the build.
- Fuzzing: malformed and truncated .als files must produce a clear error and never corrupt the repo.
- Performance benchmarks: commit and diff times tracked on the largest corpus Sets.

## 13. Risks and Open Questions

| Risk | Likelihood | Impact | Mitigation |
| --- | --- | --- | --- |
| .als schema changes between Live releases | High | High | Per-version adapters, unknown-element passthrough (preserve what we do not understand), corpus regression on each release |
| Implode produces a Set Live rejects or silently alters | Medium | Critical | Preserve unknown XML verbatim, round-trip and smoke tests, hidden backup ref before any rewrite |
| SID matching mis-identifies entities, causing spurious conflicts or lost history | Medium | High | Conservative matching, user-visible match log, stem rematch override |
| Terms of use or format-stability concerns with an undocumented format | Low | Medium | Review Ableton's license terms; avoid redistributing format specs; keep adapters thin and defensible |
| Plugin state conflicts frustrate users | High | Medium | Locks, rack-based workflow guidance, keep-both option |
| Large audio repos overwhelm hosts | Medium | Medium | Exclusions, LFS or external object store, shallow and partial clone support |
| Users edit Sets in Live while Stem rewrites them | Medium | High | Live-open detection, sibling-file checkout, never silent overwrite |

### Open questions
1. How stable are Live's numeric Ids across saves in practice? This determines how much SID matching must rely on heuristics. Needs measurement on the corpus.
2. Which volatile fields can be safely dropped without Live re-deriving something visible (zoom, selected track, scroll positions)?
3. Can warp markers and clip-level sample metadata live entirely in the .als, or do .asd files carry authoritative data that must be versioned?
4. Which hosts support Git LFS locking well enough to rely on it?
5. Do Live Packs (.alp) or Set templates have different structure we need to support for sharing instruments?

## 14. Milestones

| Phase | Scope | Exit criteria |
| --- | --- | --- |
| 0. Spike (2-3 weeks) | Parse and canonicalize .als for one Live version; answer open questions 1-3; prove explode-implode-open-in-Live on 10 real projects | Round-trip fixed point on all 10; Live opens all outputs |
| 1. Versioning (6-8 weeks) | init, commit, checkout, asset store, doctor, textconv diff | Daily use by the author on real songs; no data loss in 4 weeks of dogfooding |
| 2. Semantic diff (4-6 weeks) | Typed change list, log --entity, JSON output, minimal Review UI timeline | Diffs on corpus judged readable by 5 producers |
| 3. Merge (8-10 weeks) | Structural merge driver, SID matching hardening, conflict UX, property tests | Two-person trial: 20 merges, zero broken Sets |
| 4. Collaboration (6 weeks) | Locks, ownership, PR integration, reference bounces, Review UI audio compare | Team of 4 completes an EP on Stem |
| 5. Hardening | Multi-version support, Windows parity, perf, docs, optional Max for Live status device | Public beta |

## 15. Alternatives Considered

- Plain Git plus LFS on the project folder. Simple, but the .als is a compressed blob: every save is a full-file change, no diff, and any parallel edit is an unresolvable binary conflict. Useful only as a snapshot backup.
- Gunzip-only filter (store the raw XML). Gives line diffs and some automatic merges, but unstable ordering and ID collisions produce noisy diffs and silently corrupt merges. This is a viable stepping stone (and Phase 1 includes it as a fallback mode) but not the end state.
- Cloud snapshot services. Good for backup and sharing, but no branching, merging, or structural history.
- Export-and-share stems. The standard workflow today. Robust, but collaborators lose the editable Set: automation, devices, and MIDI are flattened.
- Live-side plugin or Max for Live integration only. Limited by Live's lack of a save/open hook and project-level API; useful as UI sugar, not as the core.

## 16. Security, Privacy, and Licensing

- Projects may contain unreleased music and licensed samples. Stem never transmits data except to remotes the user configures; no telemetry by default.
- The asset manifest records origin metadata so sample licensing can be audited; Stem does not enforce licensing.
- Remote access control is delegated to the Git host. Lock and ownership files are advisory metadata, not security boundaries.
- Plugin state blobs can embed license or serial data for some vendors; stem doctor warns on known patterns and supports an exclusion list.

## 17. Glossary

- Set: a Live document (.als).
- Canonical form: Stem's normalized, exploded representation of a Set stored in Git.
- Explode / Implode: convert a Set to canonical form and back.
- SID: stable entity ID assigned by Stem, independent of Live's numeric Ids.
- Opaque blob: binary plugin state that Stem stores byte-exact and does not interpret.
