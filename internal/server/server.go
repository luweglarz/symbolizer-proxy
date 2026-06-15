package server

import (
	"compress/gzip"
	"debug/elf"
	"encoding/binary"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/luweglarz/symbolizer-proxy/internal/exporter"
	"github.com/luweglarz/symbolizer-proxy/internal/symbolizer"
	cprofiles "go.opentelemetry.io/proto/otlp/collector/profiles/v1development"
	"google.golang.org/protobuf/proto"
)

type Server struct {
	sym      *symbolizer.Symbolizer
	exporter exporter.Exporter
	addr     string
	log      *slog.Logger
}

func New(sym *symbolizer.Symbolizer, exporter exporter.Exporter, addr string, logger *slog.Logger) *Server {
	return &Server{
		sym:      sym,
		exporter: exporter,
		addr:     addr,
		log:      logger.With("component", "server"),
	}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /debug_info", s.HandleDebugInfo)
	mux.HandleFunc("POST /v1development/profiles", s.handleProfiles)
	s.log.Info("symbolizer listening", "addr", s.addr)
	return http.ListenAndServe(s.addr, mux)
}

func (s *Server) handleProfiles(w http.ResponseWriter, r *http.Request) {
	compressBody, err := gzip.NewReader(r.Body)
	if err != nil {
		http.Error(w, "invalid gzip body: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer compressBody.Close()
	body, err := io.ReadAll(compressBody)
	defer r.Body.Close()

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	prof := &cprofiles.ExportProfilesServiceRequest{}
	if err := proto.Unmarshal(body, prof); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.sym.Symbolize(prof)

	resp := &cprofiles.ExportProfilesServiceResponse{}

	payload, err := proto.Marshal(resp)

	if err != nil {
		s.log.Error("failed to marshal response", "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(http.StatusOK)
	w.Write(payload)
	if err := s.exporter.Export(r.Context(), prof); err != nil {
		s.log.Error("export to pyroscope failed", "err", err)
	}
}

func (s *Server) HandleDebugInfo(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 20*1024*1024) // 20 MiB max request size

	err := os.MkdirAll("debug_info", 0o755)
	if err != nil {
		s.log.Error("failed to create debug_info directory", "op", "mkdir", "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpFile, err := os.CreateTemp("debug_info", "debug_info_*")
	if err != nil {
		s.log.Error("failed to create temp file", "op", "createtemp", "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()
	_, err = io.Copy(tmpFile, r.Body)
	if err != nil {
		s.log.Error("failed to read request body", "op", "copy", "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	elfFile, err := elf.NewFile(tmpFile)
	if err != nil {
		s.log.Debug("rejected non-ELF upload", "err", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer elfFile.Close()

	gnuBuildSection := elfFile.Section(".note.gnu.build-id")
	if gnuBuildSection == nil {
		s.log.Debug("rejected upload: missing .note.gnu.build-id section")
		http.Error(w, "missing .note.gnu.build-id section", http.StatusBadRequest)
		return
	}
	data, err := gnuBuildSection.Data()
	if err != nil {
		s.log.Debug("rejected upload: cannot read build-id section", "err", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if len(data) < 16 {
		http.Error(w, "invalid .note.gnu.build-id section", http.StatusBadRequest)
		return
	}

	descsz := binary.LittleEndian.Uint32(data[4:8]) // bytes 4–7 = descsz
	if int(descsz) > len(data)-16 {
		http.Error(w, "invalid build-id note", http.StatusBadRequest)
		return
	}
	id := hex.EncodeToString(data[16 : 16+descsz])

	err = os.Rename(tmpFile.Name(), "debug_info/"+id)
	if err != nil {
		s.log.Error("failed to store debug info", "op", "rename", "buildid", id, "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.log.Info("stored debug info", "buildid", id)
}
