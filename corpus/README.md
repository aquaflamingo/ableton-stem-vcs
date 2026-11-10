# Corpus

Real Ableton Live projects used to validate Stem. **Contents are gitignored** —
Sets and audio may contain unreleased music or licensed samples, and the `.gitignore`
excludes `corpus/*/` on purpose. Only this README is committed.

## Current

| Project | Live version | Notes |
| --- | --- | --- |
| `supreme-something/` | 11.3.43 | 20 tracks (6 MIDI, 4 audio, 2 group, 8 return), Max for Live devices, freeze + reverse audio, one `.asd` sidecar. Includes an earlier `Backup/` autosave used for Id-stability and ordering comparisons. |

Measured facts about this Set are in [../docs/FORMAT-NOTES.md](../docs/FORMAT-NOTES.md).

## What we still need

Per [../docs/IMPLEMENTATION_PLAN.md](../docs/IMPLEMENTATION_PLAN.md):

- size spread: small, medium, huge (200 MB+ of XML)
- character: session-heavy, arrangement-heavy, rack-heavy
- **plugin-heavy**: VST2, VST3, AU (the current Set has none)
- Max-for-Live-heavy
- at least one **Live 12** Set
- many groups and returns, deep nested racks

## Adding a project

1. Drop the whole project folder under `corpus/<slug>/` (keep `Samples/` and
   `Backup/` as exported by Live).
2. Add a row to the table above: slug, Live version, a one-line description.
3. Record the SHA-256 of each `.als` in a sibling `CHECKSUMS` file so we can detect
   accidental corruption.

Do not `git add` corpus contents; they are ignored by design. If a fixture is small
and license-clear, put a *synthetic* version under `testdata/` instead.

## Synthesizing Sets

For exhaustive and fuzz testing we also generate Sets programmatically (many
tracks, deep racks, dense automation). Those are committed under `testdata/` and are
clearly marked as synthetic.
