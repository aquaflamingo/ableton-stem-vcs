# FORMAT-NOTES.md — Observations from a real Live Set

Evidence gathered on 2026-11-10 by decompressing and inspecting a real project
(`corpus/supreme-something/`, Live 11.3.43) and its earlier autosave in
`Backup/`. These notes exist to replace assumptions in TDD v0.1 §3 with measured
facts. Every claim below is reproduced by a one-off script; the numbers are from
the main `.als` unless noted.

## Sample under inspection

| Field | Value |
| --- | --- |
| Compressed size | 490,027 bytes (gzip, magic `1f 8b 08`) |
| Decompressed XML | 5,361,806 bytes (~5.1 MB) |
| Root element | `<Ableton MajorVersion="5" MinorVersion="11.0_11300" SchemaChangeCount="7" Creator="Ableton Live 11.3.43" Revision="b466d4…">` |
| Structure | `<Ableton><LiveSet><NextPointeeId/><OverwriteProtectionNumber/><LomId/>…<Tracks>…` |
| Tracks | 6 MIDI, 4 audio, 2 group, 8 return (20 total) |
| Backup comparison | `<Ableton MinorVersion="11.0_11300" SchemaChangeCount="7">`, 4,838,505 bytes |

Two facts immediately matter. The root element carries a **`SchemaChangeCount`**
and a **`Revision`** hash in addition to `MajorVersion`/`MinorVersion`. And the
compressed Set is small (0.5 MB) while its XML is 10× larger: the gzip blob hides
a 5 MB text file that no line-based tool can see.

## 1. Identity is not one counter — it is many ID spaces

TDD v0.1 §3 says "Numeric Id attributes … IDs are allocated per Set from
counters." The real file has **13,536 elements carrying an `Id` attribute across
more than 140 distinct element names**. The important spaces, with counts:

| Element | Instances | What it identifies |
| --- | --- | --- |
| `AutomationTarget` | 6,413 | Addressable target for a parameter's automation |
| `ModulationTarget` | 4,142 | Addressable target for modulation |
| `Pointee` | 282 | A reference *to* a target by Id |
| `TrackSendHolder` | 160 | Per-track send into a return |
| `ClipSlot` | 160 | Session grid cell |
| `WarpMarker` | 237 | A warp marker inside an audio clip |
| `Scene` | 8 | Scene row |
| `FileRef`, `SampleRef`, `SourceContext`, `BranchSourceContext` | many | Sample provenance chain |
| `MxDBlob`, `MxDIdRef`, `MxDEnumParameter`, `MxD*Parameter` | many | Max for Live device state and internal refs |
| `ControllerTargets.0` … `ControllerTargets.128` | 129 | MIDI remote targets, one per CC |
| `AbletonDefaultPresetRef`, `MxDPatchRef`, `FilePresetRef` | many | Preset pointers |

`<NextPointeeId Value="52447"/>` lives in `LiveSet` and is a **separate monotonic
counter** (the backup has `51491`). `Pointee` references reach `52427`, so the
pointee space is far larger than any track or clip Id. Track Ids are not
contiguous and not sorted: `60, 43, 54, 59, 55, 62, 57, 58, 56, 52, 53, 61` then
returns `2, 34, 35, 36, 37, 49, 40, 41`.

**Consequence:** the design cannot model "the next Id" as a single counter. Every
ID space needs its own allocator and its own reference-rewriting pass on implode.
`AutomationTarget`/`ModulationTarget`/`Pointee` are the ones that cross-reference
into automation lanes, and `TrackSendHolder`/`Scene`/`ClipSlot` are positioned
within their parent. This directly informs CH-N3 in the critique.

## 2. Live's numeric Ids are stable across saves

Open question 1 asked how stable Ids are. Comparing the main Set with its earlier
autosave in `Backup/`:

| Track name | Id in backup | Id in main |
| --- | --- | --- |
| BUS_KICK (group) | 60 | 60 |
| kick | 43 | 43 |
| kick | 54 | 54 |
| perc (group) | 59 | 59 |
| 5-LCD2_Perc_Tamb_75 | 55 | 55 |
| 7-PML_DPV3_SHAKER_008 | 57 | 57 |
| 8-PML_DPV7_SV_Shakerloop_008_125BPM | 58 | 58 |
| 9-LCD2_Snare_16 | 56 | 56 |
| 10-03 Thank You Lord | 52 | 52 |
| bass | 53 | 53 |
| A-drum-verb … H-para-comp (returns) | 2,34,35,36,37,49,40,41 | identical |

Every Id present in the backup is **unchanged** in the later save. The main Set
simply added two tracks (`12-Crash 909` = 61, `6-04_OpenHat_Long_909` = 62),
which received fresh, previously-unused Ids. The return-track set is identical.

This is a single project and a single save interval, so it is not proof for all
cases — but it is strong evidence that Live assigns an Id once and keeps it for
the life of the entity, allocating new Ids upward from its counters.

**Consequence:** the exact Live Id, scoped by type and parent, is a far stronger
identity signal than TDD v0.1 §6.1 assumed. See CH-B3 in the critique.

## 3. Sample references carry absolute paths and mutable metadata

A `SampleRef` looks like this (abridged):

```xml
<SampleRef>
  <FileRef>
    <RelativePathType Value="5" />
    <RelativePath Value="Samples/One Shots/Drums/Kick/Kick 909 1.aif" />
    <Path Value="/Applications/Ableton Live 11 Suite.app/Contents/App-Resources/Core Library/Samples/One Shots/Drums/Kick/Kick 909 1.aif" />
    <Type Value="2" />
    <LivePackName Value="Core Library" />
    <LivePackId Value="www.ableton.com/0" />
    <OriginalFileSize Value="201952" />
    <OriginalCrc Value="649" />
  </FileRef>
  <LastModDate Value="1663760958" />
  <SourceContext><SourceContext Id="0"><OriginalFileRef><FileRef Id="0">…</FileRef>…
</SampleRef>
```

Each reference carries both a **relative** and an **absolute** path, plus
`OriginalFileSize`, `OriginalCrc` and `LastModDate`. `RelativePathType` and
`LivePackName`/`LivePackId` distinguish core-library files from user files.

**Consequences:**
- Pathportability (TDD v0.1 §8) is real: `/Applications/Ableton Live 11 Suite.app/…`
  cannot exist on another machine.
- `LastModDate`/`OriginalCrc` mean a checkout that rewrites bytes or mtimes can
  make Live believe a sample changed. See CH-B5.
- The `SourceContext`/`BranchSourceContext` chain stores *provenance* (which
  original an edited copy came from). It is itself Id-bearing and must survive a
  round trip.
- This Set has 352 `RelativePath`/`Path` pairs and 13 `SampleRef`s, and it
  references files inside the Ableton Core Library and a Live Pack — external
  references are the norm, not the exception.

## 4. Opaque state is inline hex text, not a sidecar file

A Max for Live device stores its state as hex-encoded text inside the XML:

```xml
<MxDBlob Id="2">
  <Blob>
    7B0A09225461726765744D61785B31315D22203A205B20313030205D2C0A…
```

Decoded, the blob begins `{\n\t"TargetMax[11]" : [ 100 ],\n\t"TargetMax[12]" :
[ 50 ],…` — it is JSON, but Max devices generally are not. The sample has 10
`MxDBlob`s and 10 `MxDeviceAudioEffect`s.

**Consequence:** the "binary buffer as hex text" property from TDD v0.1 §3 is
confirmed and is *inside* the XML, so it will pollute line diffs unless extracted
to sidecar files. It is also not necessarily binary — some blobs are text — so the
sidecar strategy should be content-agnostic: treat every `Blob` value as an
opaque byte string keyed by its owning entity's SID.

## 5. The Set is dense with per-parameter automation machinery

`LockEnvelope` appears **11,466** times and `OnsetEvent` **10,016** times;
`Min`/`Max`/`MidiControllerRange` each appear thousands of times. There are 6,413
automation targets and 4,142 modulation targets but only 4 `AutomationEnvelope`
elements and 25 of each named `*ModulationTarget` (e.g. `VolumeModulationTarget`).

**Consequence:** most of the file is boilerplate target/range scaffolding
attached to every parameter, not user edits. A canonical form that stores one
node per parameter will produce canonical trees far smaller than the XML but must
still reproduce all 6,413 target Ids deterministically on implode. `OnsetEvent`
is transient audio analysis embedded in the Set — the same data also exists in
`.asd` sidecars (see §7), confirming open question 3 is not yet settled.

## 6. Ordering looks meaningful and stable, but was not proven noisy

In both the main Set and the backup, tracks appear in the same order (a group
track followed by its members), and this order is **not** Id-sorted. That order
is the arrangement/track order a user sees. Between the two saves, 94.4% of
decompressed lines are byte-identical; 6,533 lines were removed/changed and
14,630 added.

We **cannot** separate "diff noise from re-saving" from "real edits" with these
two files, because the backup is a genuinely earlier state (two fewer tracks).
So TDD v0.1's claim that "element ordering and whitespace [are] not guaranteed
stable across saves" is **unproven**. It must be tested properly in the spike:
save the *same* Set twice with no edits and compare byte-for-byte.

**Consequence:** do not re-sort children blindly (CH-A5). Measure first. The
`LockEnvelope`/`OnsetEvent` repetition shows that even a benign re-save could
reorder thousands of nodes if Live's writer is not stable, so this is the single
most important measurement for diff quality.

## 7. Other observed facts

- The project ships a `.asd` sidecar (`Crash 909 R.aif.asd`, 6,806 bytes) next to
  a Processed/Reverse audio file, so analysis data exists both inline and as a
  sidecar. The `.asd` is gzip too (Live 11 uses an XML-ish analysis format).
- Freeze is used: the Set contains 158 mentions of `Freeze`, and
  `Samples/Processed/Reverse/` holds the rendered output of a Reverse operation.
- No VST/AU plugin devices were present in this Set (`VstPluginInfo` count 0);
  all devices are native Live devices or Max for Live. A plugin-heavy corpus case
  is still missing.
- The `Ableton Project Info/` directory is empty in this export; the `Icon` file
  at project root is a Finder icon, not part of the Set.

## What this sample does and does not settle

| TDD open question / assumption | Status after this sample |
| --- | --- |
| Ids are one counter (v0.1 §3) | **Refuted.** Many independent Id spaces. |
| Ids are unstable and reassigned (v0.1 §6.1) | **Weakened.** Ids were stable across two saves. |
| Ordering/whitespace unstable across saves (§3) | **Unproven.** Needs a controlled no-edit re-save. |
| Warp/analysis data lives in .als *and* .asd (Q3) | **Both exist.** Still needs the authoritative-source test. |
| Sample refs carry abs paths + metadata (§8) | **Confirmed.** Path, CRC, size, LastModDate all present. |
| Inline opaque hex blobs (§3) | **Confirmed** (MxDBlob). |
| Plugin blobs are the opaque case (§8) | **Not tested here** — this Set has no VST/AU. |
| Live version attributes on root (§3) | **Confirmed**, plus SchemaChangeCount and Revision. |
