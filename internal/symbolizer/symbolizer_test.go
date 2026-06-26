package symbolizer

import (
	"debug/elf"
	"log/slog"
	"testing"

	cprofiles "go.opentelemetry.io/proto/otlp/collector/profiles/v1development"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	profilespb "go.opentelemetry.io/proto/otlp/profiles/v1development"
)

func TestNormalizeAddr(t *testing.T) {
	m := &profilespb.Mapping{MemoryStart: 0x1000, FileOffset: 0x200}
	if got := normalizeAddr(0x1500, m); got != 0x700 {
		t.Fatalf("normalizeAddr = 0x%x, want 0x700", got)
	}
}

func profWithBuildID(key, value string) *cprofiles.ExportProfilesServiceRequest {
	return &cprofiles.ExportProfilesServiceRequest{
		Dictionary: &profilespb.ProfilesDictionary{
			StringTable:  []string{key},
			MappingTable: []*profilespb.Mapping{{AttributeIndices: []int32{0}}},
			AttributeTable: []*profilespb.KeyValueAndUnit{{
				KeyStrindex: 0,
				Value:       &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}},
			}},
		},
	}
}

func TestGetBuildID(t *testing.T) {
	prof := profWithBuildID("process.executable.build_id.gnu", "deadbeef")
	if got := getBuildID(prof, 0); got != "deadbeef" {
		t.Fatalf("getBuildID = %q, want deadbeef", got)
	}
}

func TestGetBuildIDMissing(t *testing.T) {
	prof := profWithBuildID("process.executable.build_id.htlhash", "deadbeef")
	if got := getBuildID(prof, 0); got != "" {
		t.Fatalf("getBuildID = %q, want empty", got)
	}
}

type fakeSource struct {
	symbols []elf.Symbol
}

func (fs *fakeSource) Symbols(buildID string) ([]elf.Symbol, error) {
	return fs.symbols, nil
}

func testResolveLocation(t *testing.T, buildID string, addr uint64) {
	prof := profWithBuildID("process.executable.build_id.gnu", buildID)
	loc := &profilespb.Location{MappingIndex: 0, Address: addr}
	symSource := &fakeSource{symbols: []elf.Symbol{{Name: "foo", Value: 0x1000, Size: 0x10}}}
	s := New(slog.Default(), symSource)
	resolved := make(map[string]int32)
	s.resolveLocation(prof, loc, resolved)
	if len(resolved) == 0 {
		t.Fatalf("resolveLocation failed to resolve any symbols for buildID %q and address 0x%x", buildID, addr)
	}
}

func TestResolveLocation(t *testing.T) {
	testResolveLocation(t, "foo", 0x1000)
}

func TestResolveLocationDemangles(t *testing.T) {
	tests := []struct {
		name           string
		symName        string // what's in the ELF symbol table
		wantName       string // expected human-readable name (NameStrindex)
		wantSystemName string // expected raw linker name (SystemNameStrindex)
	}{
		{
			name:           "mangled C++ symbol",
			symName:        "_Z3fooi",
			wantName:       "foo(int)",
			wantSystemName: "_Z3fooi",
		},
		{
			name:           "plain unmangled symbol",
			symName:        "main.main",
			wantName:       "main.main",
			wantSystemName: "main.main",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prof := profWithBuildID("process.executable.build_id.gnu", "testbuild")
			loc := &profilespb.Location{MappingIndex: 0, Address: 0x1000}
			symSource := &fakeSource{symbols: []elf.Symbol{{Name: tc.symName, Value: 0x1000, Size: 0x10}}}
			s := New(slog.Default(), symSource)

			s.resolveLocation(prof, loc, make(map[string]int32))

			if len(loc.Lines) != 1 {
				t.Fatalf("expected 1 line appended, got %d", len(loc.Lines))
			}
			fn := prof.Dictionary.FunctionTable[loc.Lines[0].FunctionIndex]
			gotName := prof.Dictionary.StringTable[fn.NameStrindex]
			gotSystemName := prof.Dictionary.StringTable[fn.SystemNameStrindex]

			if gotName != tc.wantName {
				t.Errorf("NameStrindex => %q, want %q", gotName, tc.wantName)
			}
			if gotSystemName != tc.wantSystemName {
				t.Errorf("SystemNameStrindex => %q, want %q", gotSystemName, tc.wantSystemName)
			}
		})
	}
}
