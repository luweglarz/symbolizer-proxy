package symbolizer

import (
	"debug/elf"
	"flag"
	"log/slog"
	"unsafe"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/ianlancetaylor/demangle"
	cprofiles "go.opentelemetry.io/proto/otlp/collector/profiles/v1development"
	profilespb "go.opentelemetry.io/proto/otlp/profiles/v1development"
)

type symbolSource interface {
	Symbols(buildID string) ([]elf.Symbol, error)
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

type Config struct {
	CacheNumCounters int64
	CacheSizeMB      int64
}

func (c *Config) RegisterFlags(fs *flag.FlagSet) {
	fs.Int64Var(&c.CacheNumCounters, "symbolizer.cache_num_counters", 1e7, "symbolizer cache number of counters")
	fs.Int64Var(&c.CacheSizeMB, "symbolizer.cache_size_mb", 2048, "symbolizer cache size in MB")
}

type Symbolizer struct {
	log      *slog.Logger
	source   symbolSource
	elfCache *ristretto.Cache[string, []elf.Symbol]
	config   Config
}

func New(logger *slog.Logger, source symbolSource, config Config) *Symbolizer {
	s := &Symbolizer{}
	s.log = logger.With("component", "symbolizer")
	s.source = source
	var err error
	s.elfCache, err = ristretto.NewCache(&ristretto.Config[string, []elf.Symbol]{
		NumCounters: config.CacheNumCounters,
		MaxCost:     config.CacheSizeMB * 1024 * 1024, // cache size in MB.
		BufferItems: 64,
	})

	if err != nil {
		s.log.Error("failed to create cache", "error", err)
		return nil
	}
	return s
}

func (s *Symbolizer) Close() {
	s.elfCache.Close()
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

	symbs, ok := s.elfCache.Get(buildID)
	if !ok {
		var err error
		symbs, err = s.source.Symbols(buildID)
		s.elfCache.Set(buildID, symbs, int64(unsafe.Sizeof(elf.Symbol{}))*int64(len(symbs)))
		if err != nil {
			return
		}
	}
	var matched *elf.Symbol
	normalizedAddr := normalizeAddr(loc.Address, prof.Dictionary.MappingTable[loc.MappingIndex])
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
	fnIdx, ok := resolved[mapKey]
	if !ok {
		// TODO: consider caching the demangled name to avoid repeated demangling for the same symbol
		// TODO: consider possibility to configure filter options for demangling
		demangled := demangle.Filter(matched.Name)

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
	loc.Lines = append(loc.Lines, &profilespb.Line{
		FunctionIndex: fnIdx,
		Line:          0,
	})
}

func (s *Symbolizer) symbolizeLocations(prof *cprofiles.ExportProfilesServiceRequest, stckIdx int32, resolved map[string]int32) {
	stckTable := prof.Dictionary.StackTable[stckIdx]
	for _, locationIdx := range stckTable.LocationIndices {
		loc := prof.Dictionary.LocationTable[locationIdx]
		if len(loc.Lines) == 0 {
			s.resolveLocation(prof, loc, resolved)
		}
	}
}

func (s *Symbolizer) Symbolize(prof *cprofiles.ExportProfilesServiceRequest) {
	if prof == nil || prof.Dictionary == nil || prof.ResourceProfiles == nil || len(prof.Dictionary.StackTable) == 0 {
		s.log.Debug("no stack traces to symbolize")
		return
	}
	resolved := make(map[string]int32)
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
