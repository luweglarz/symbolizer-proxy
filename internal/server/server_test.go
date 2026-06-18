package server

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestHandleDebugInfo drives the upload handler over a real HTTP server with
// each generated fixture (run `make fixtures` first). It checks the happy path
// (valid ELF -> stored under its build-id) and the rejection paths (bad input
// -> 4xx/5xx, nothing stored, no panic).
func TestHandleDebugInfo(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	testdata := filepath.Join(wd, "testdata")

	cases := []struct {
		name    string
		fixture string
		wantOK  bool   // true => 200 + stored; false => rejected + nothing stored
		wantKey string // exact storage key to expect ("" => don't pin the name)
	}{
		// Pinned build-id (linker -B): deterministic, so we can assert the exact key.
		{"pinned build-id", "dummyapp.fixedid", true, "00112233445566778899aabbccddeeff00112233"},
		// Toolchain-derived build-ids drift across Go versions, so don't pin the name.
		{"normal binary", "dummyapp.normal", true, ""},
		{"stripped binary", "dummyapp.stripped", true, ""},
		// Not an ELF at all: must be rejected cleanly, with no file left behind.
		{"non-ELF rejected", "not-an-elf.bin", false, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join(testdata, tc.fixture))
			if err != nil {
				t.Fatalf("read fixture %s: %v (did you run `make fixtures`?)", tc.fixture, err)
			}

			// The handler stores to a relative "debug_info/" dir; run it in an
			// isolated cwd so each case's output is separate and discarded.
			t.Chdir(t.TempDir())

			srv := &Server{log: slog.New(slog.DiscardHandler)}
			ts := httptest.NewServer(http.HandlerFunc(srv.HandleDebugInfo))
			defer ts.Close()

			resp, err := http.Post(ts.URL, "application/octet-stream", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("POST: %v", err)
			}
			resp.Body.Close()

			if tc.wantOK {
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("status = %d, want 200", resp.StatusCode)
				}
				assertStored(t, body, tc.wantKey)
			} else {
				if resp.StatusCode < 400 {
					t.Fatalf("status = %d, want >= 400 for invalid upload", resp.StatusCode)
				}
				assertNothingStored(t)
			}
		})
	}
}

// assertStored verifies exactly one file landed in debug_info/, that its bytes
// match what we uploaded, and (when key != "") that it was stored under key.
func assertStored(t *testing.T, want []byte, key string) {
	t.Helper()
	entries, err := os.ReadDir("debug_info")
	if err != nil {
		t.Fatalf("debug_info not created: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("debug_info has %d entries, want exactly 1", len(entries))
	}
	name := entries[0].Name()
	if key != "" && name != key {
		t.Fatalf("stored under %q, want %q", name, key)
	}
	got, err := os.ReadFile(filepath.Join("debug_info", name))
	if err != nil {
		t.Fatalf("read stored file: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("stored %d bytes but uploaded %d; content differs", len(got), len(want))
	}
}

// assertNothingStored verifies a rejected upload left no file behind (the temp
// file must have been cleaned up; the dir may or may not exist).
func assertNothingStored(t *testing.T) {
	t.Helper()
	entries, err := os.ReadDir("debug_info")
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		t.Fatalf("stat debug_info: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("debug_info has %d entries, want 0 after a rejected upload", len(entries))
	}
}
