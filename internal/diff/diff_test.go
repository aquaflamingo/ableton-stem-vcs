package diff

import (
	"strings"
	"testing"

	"github.com/aquaflamingo/stem/internal/als"
)

const baseXML = `<Ableton MinorVersion="11.0_11300" Creator="Ableton Live 11.3.43">
	<LiveSet>
		<NextPointeeId Value="100" />
		<Tracks>
			<MidiTrack Id="2">
				<Name><EffectiveName Value="Bass" /></Name>
				<Devices><Eq8 Id="0" /></Devices>
			</MidiTrack>
		</Tracks>
		<MasterTrack />
		<Tempo><Manual Value="120" /></Tempo>
	</LiveSet>
</Ableton>`

const changedXML = `<Ableton MinorVersion="11.0_11300" Creator="Ableton Live 11.3.43">
	<LiveSet>
		<NextPointeeId Value="120" />
		<Tracks>
			<MidiTrack Id="2">
				<Name><EffectiveName Value="Sub" /></Name>
				<Devices><Eq8 Id="0" /><Compressor2 Id="1" /></Devices>
			</MidiTrack>
			<AudioTrack Id="5">
				<Name><EffectiveName Value="Vocals" /></Name>
			</AudioTrack>
		</Tracks>
		<MasterTrack />
		<Tempo><Manual Value="124" /></Tempo>
	</LiveSet>
</Ableton>`

func mustParse(t *testing.T, s string) *als.Doc {
	t.Helper()
	d, err := als.Parse([]byte(s))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return d
}

func TestCompare(t *testing.T) {
	a := mustParse(t, baseXML)
	b := mustParse(t, changedXML)
	changes, err := Compare(a, b)
	if err != nil {
		t.Fatal(err)
	}
	out := Format(changes)

	wants := []string{
		"+ track 'Vocals' (AudioTrack)",
		"renamed 'Bass' -> 'Sub'",
		"tempo 120 -> 124 BPM",
		"device 'Compressor2'",
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("diff missing %q in:\n%s", w, out)
		}
	}
}

func TestCompareIdentical(t *testing.T) {
	a := mustParse(t, baseXML)
	b := mustParse(t, baseXML)
	changes, err := Compare(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("expected no changes, got %d:\n%s", len(changes), Format(changes))
	}
}
