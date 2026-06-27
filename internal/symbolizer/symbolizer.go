package symbolizer

import (
	"debug/elf"
	"log/slog"

	"github.com/ianlancetaylor/demangle"
	cprofiles "go.opentelemetry.io/proto/otlp/collector/profiles/v1development"
	profilespb "go.opentelemetry.io/proto/otlp/profiles/v1development"
)

type symbolSource interface {
	Symbols(buildID string) ([]elf.Symbol, error)
}

type Symbolizer struct {
	log    *slog.Logger
	source symbolSource
}

type FileSource struct{}

func (fs *FileSource) Symbols(buildID string) ([]elf.Symbol, error) {
	file, err := elf.Open("debug_info/" + buildID)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return file.Symbols()
}

func New(logger *slog.Logger, source symbolSource) *Symbolizer {
	s := &Symbolizer{}
	s.log = logger.With("component", "symbolizer")
	s.source = source
	return s
}

func getBuildID(prof *cprofiles.ExportProfilesServiceRequest, idx int32) string {
	mapping := prof.Dictionary.MappingTable[idx]
	for _, attrIdx := range mapping.AttributeIndices {
		attr := prof.Dictionary.AttributeTable[attrIdx]
		key := prof.Dictionary.StringTable[attr.KeyStrindex]
		if key == "process.executable.build_id.gnu" {
			return attr.Value.GetStringValue()
		}
	}
	return ""
}

func normalizeAddr(addr uint64, mapping *profilespb.Mapping) uint64 {
	// normalize the address by subtracting the mapping's memory start and adding the file offset
	// example : if the mapping's memory start is 0x1000 and the file offset is 0x200, then an address of 0x1200 would be normalized to 0x400 (0x1200 - 0x1000 + 0x200)
	return addr - mapping.MemoryStart + mapping.FileOffset
}

func (s *Symbolizer) resolveLocation(prof *cprofiles.ExportProfilesServiceRequest, loc *profilespb.Location, resolved map[string]int32) {
	buildID := getBuildID(prof, loc.MappingIndex)
	if buildID == "" {
		return
	}

	symbs, err := s.source.Symbols(buildID)
	if err != nil {
		//s.log.Warn("failed to read symbols from ELF file", "build_id", buildID, "error", err)
		return
	}
	var matched *elf.Symbol
	normalizedAddr := normalizeAddr(loc.Address, prof.Dictionary.MappingTable[loc.MappingIndex])
	// loop through symbols to find one that matches the location's address
	for _, sym := range symbs {
		if sym.Value <= normalizedAddr && normalizedAddr < sym.Value+sym.Size {
			matched = &sym
			break
		}
	}
	if matched == nil {
		return
	}
	mapKey := buildID + ":" + matched.Name
	// check if the symbol has already been resolved
	fnIdx, ok := resolved[mapKey]
	if !ok {
		// TODO: consider caching the demangled name to avoid repeated demangling for the same symbol
		// TODO: consider possibility to configure filter options for demangling
		demangled := demangle.Filter(matched.Name)
		// fill dictionnary
		prof.Dictionary.StringTable = append(prof.Dictionary.StringTable, matched.Name)
		nameIdx := int32(len(prof.Dictionary.StringTable) - 1)
		systemNameIdx := nameIdx
		if demangled != matched.Name {
			prof.Dictionary.StringTable = append(prof.Dictionary.StringTable, demangled)
			systemNameIdx = int32(len(prof.Dictionary.StringTable) - 1)
		}
		prof.Dictionary.FunctionTable = append(prof.Dictionary.FunctionTable, &profilespb.Function{ // add a Function to the FunctionTable for this symbol
			NameStrindex:       systemNameIdx,
			SystemNameStrindex: nameIdx,
			FilenameStrindex:   0,
			StartLine:          0,
		})
		fnIdx = int32(len(prof.Dictionary.FunctionTable) - 1)
		resolved[mapKey] = fnIdx
	}
	// add a Line to the Location for this symbol
	loc.Lines = append(loc.Lines, &profilespb.Line{
		FunctionIndex: fnIdx,
		Line:          0,
	})
}

func (s *Symbolizer) symbolizeLocations(prof *cprofiles.ExportProfilesServiceRequest, stckIdx int32, resolved map[string]int32) {
	stckTable := prof.Dictionary.StackTable[stckIdx]
	// loop through the stack's locations
	for _, locationIdx := range stckTable.LocationIndices {
		loc := prof.Dictionary.LocationTable[locationIdx]
		if len(loc.Lines) == 0 { // need symbolization
			s.resolveLocation(prof, loc, resolved)
		}
	}
}

func (s *Symbolizer) Symbolize(prof *cprofiles.ExportProfilesServiceRequest) {
	if prof == nil || prof.Dictionary == nil || prof.ResourceProfiles == nil || len(prof.Dictionary.StackTable) == 0 {
		s.log.Debug("no stack traces to symbolize")
		return
	}
	resolved := make(map[string]int32) // map of buildID to set of resolved symbol indices in the dictionary
	for _, r := range prof.ResourceProfiles {
		for _, sp := range r.ScopeProfiles {
			for _, p := range sp.Profiles {
				for _, samp := range p.Samples {
					s.symbolizeLocations(prof, samp.StackIndex, resolved)
				}
			}
		}
	}

}
