package symbolizer

import (
	"debug/elf"
	"log/slog"

	cprofiles "go.opentelemetry.io/proto/otlp/collector/profiles/v1development"
	profilespb "go.opentelemetry.io/proto/otlp/profiles/v1development"
)

type Symbolizer struct {
	log *slog.Logger
}

func New(logger *slog.Logger) *Symbolizer {
	s := &Symbolizer{}
	s.log = logger.With("component", "symbolizer")
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
	return addr - mapping.MemoryStart + mapping.FileOffset
}

func (s *Symbolizer) resolveLocation(prof *cprofiles.ExportProfilesServiceRequest, loc *profilespb.Location) {
	buildID := getBuildID(prof, loc.MappingIndex)
	if buildID == "" {
		return
	}

	file, err := elf.Open("debug_info/" + buildID)
	if err != nil {
		return
	}
	defer file.Close()
	symbs, err := file.Symbols()
	if err != nil {
		s.log.Warn("failed to read symbols from ELF file", "build_id", buildID, "error", err)
		return
	}
	s.log.Debug("opened ELF file", "build_id", buildID, "num_symbols", len(symbs), "location_address", loc.Address, "mapping_index", loc.MappingIndex)
	for _, sym := range symbs { // loop through symbols to find one that matches the location's address
		normalizedAddr := normalizeAddr(loc.Address, prof.Dictionary.MappingTable[loc.MappingIndex])
		// TODO: remove logging of symbol information once symbolization is working correctly
		s.log.Debug("checking symbol", "symbol_name", sym.Name, "symbol_value", sym.Value, "symbol_size", sym.Size, "normalized_location_address", normalizedAddr)
		if sym.Value <= normalizedAddr && normalizedAddr < sym.Value+sym.Size {
			prof.Dictionary.StringTable = append(prof.Dictionary.StringTable, sym.Name) // create a new entry in the string table for this symbol's name
			nameIdx := int32(len(prof.Dictionary.StringTable) - 1)
			s.log.Info("symbolized location", "location_address", loc.Address, "mapping_index", loc.MappingIndex, "symbol_name", prof.Dictionary.StringTable[nameIdx])
			// TODO: avoid bloating dictionary with duplicate function entries for the same symbol name mayb consider using a map to track existing function names and their indices
			prof.Dictionary.FunctionTable = append(prof.Dictionary.FunctionTable, &profilespb.Function{ // add a Function to the FunctionTable for this symbol
				NameStrindex:       nameIdx,
				SystemNameStrindex: nameIdx,
				FilenameStrindex:   0,
				StartLine:          0,
			})
			loc.Lines = append(loc.Lines, &profilespb.Line{ // add a Line to the Location for this symbol
				FunctionIndex: int32(len(prof.Dictionary.FunctionTable) - 1),
			})
			return
		}
	}

}

func (s *Symbolizer) symbolizeLocations(prof *cprofiles.ExportProfilesServiceRequest, stckIdx int32) {
	stckTable := prof.Dictionary.StackTable[stckIdx]
	for _, locationIdx := range stckTable.LocationIndices { // loop through the stack's locations
		loc := prof.Dictionary.LocationTable[locationIdx]
		if len(loc.Lines) == 0 { // need symbolization
			s.resolveLocation(prof, loc)
		}
	}
}

func (s *Symbolizer) Symbolize(prof *cprofiles.ExportProfilesServiceRequest) {
	if prof == nil || prof.Dictionary == nil || prof.ResourceProfiles == nil || len(prof.Dictionary.StackTable) == 0 {
		s.log.Debug("no stack traces to symbolize")
		return
	}
	for _, r := range prof.ResourceProfiles {
		for _, sp := range r.ScopeProfiles {
			for _, p := range sp.Profiles {
				for _, samp := range p.Samples { // samples have stack traces
					s.symbolizeLocations(prof, samp.StackIndex)
				}
			}
		}
	}

}
