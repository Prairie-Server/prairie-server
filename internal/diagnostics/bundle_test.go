package diagnostics

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"io"
	"strings"
	"testing"
)

type testArchiveEntry struct {
	name     string
	body     []byte
	typeflag byte
}

func TestValidateBundle(t *testing.T) {
	validEntries := []testArchiveEntry{
		{name: "manifest.json", body: []byte(`{"schema_version":1}`)},
		{name: "logs.jsonl", body: []byte("{}\n")},
	}

	tests := []struct {
		name        string
		entries     []testArchiveEntry
		limits      BundleLimits
		mutate      func([]byte) []byte
		wantErr     error
		wantEntries string
	}{
		{
			name:    "happy path",
			entries: validEntries,
		},
		{
			name: "wrong first entry",
			entries: []testArchiveEntry{
				{name: "device.json", body: []byte(`{}`)},
				{name: "manifest.json", body: []byte(`{}`)},
			},
			wantErr: ErrInvalidBundle,
		},
		{
			name: "disallowed name",
			entries: []testArchiveEntry{
				{name: "manifest.json", body: []byte(`{}`)},
				{name: "extra.txt", body: []byte("nope")},
			},
			wantErr: ErrInvalidBundle,
		},
		{
			name: "traversal name",
			entries: []testArchiveEntry{
				{name: "manifest.json", body: []byte(`{}`)},
				{name: "../logs.jsonl", body: []byte("nope")},
			},
			wantErr: ErrInvalidBundle,
		},
		{
			name: "ratio bomb",
			entries: []testArchiveEntry{
				{name: "manifest.json", body: []byte(`{}`)},
				// Valid newline-delimited JSON objects that still compress far
				// beyond the ratio limit, so the ratio guard trips before the
				// content validator can pass judgement on every line.
				{name: "logs.jsonl", body: bytes.Repeat([]byte("{}\n"), 400*1024)},
			},
			limits: BundleLimits{
				MaxUncompressedBytes: 4 * 1024 * 1024,
			},
			wantErr: ErrCompressionRatio,
		},
		{
			name: "rejects malformed device.json",
			entries: []testArchiveEntry{
				{name: "manifest.json", body: []byte(`{}`)},
				{name: "device.json", body: []byte(`{"identity": }`)},
			},
			wantErr: ErrInvalidBundle,
		},
		{
			name: "rejects non-object device.json",
			entries: []testArchiveEntry{
				{name: "manifest.json", body: []byte(`{}`)},
				{name: "device.json", body: []byte(`[1,2,3]`)},
			},
			wantErr: ErrInvalidBundle,
		},
		{
			name: "rejects trailing data in device.json",
			entries: []testArchiveEntry{
				{name: "manifest.json", body: []byte(`{}`)},
				{name: "device.json", body: []byte(`{}{}`)},
			},
			wantErr: ErrInvalidBundle,
		},
		{
			name: "rejects malformed logs.jsonl line",
			entries: []testArchiveEntry{
				{name: "manifest.json", body: []byte(`{}`)},
				{name: "logs.jsonl", body: []byte("{}\nnot json\n")},
			},
			wantErr: ErrInvalidBundle,
		},
		{
			name: "rejects non-object logs.jsonl line",
			entries: []testArchiveEntry{
				{name: "manifest.json", body: []byte(`{}`)},
				{name: "logs.jsonl", body: []byte("{}\n[1,2]\n")},
			},
			wantErr: ErrInvalidBundle,
		},
		{
			name: "accepts well-formed device.json and crash json",
			entries: []testArchiveEntry{
				{name: "manifest.json", body: []byte(`{}`)},
				{name: "device.json", body: []byte(`{"identity":{"model":"Pixel"}}` + "\n")},
				{name: "logs.jsonl", body: []byte(`{"lvl":"I","msg":"hi"}` + "\n")},
				{name: "crash/summary.json", body: []byte(`{"summary":"boom"}`)},
				{name: "crash/tombstone.pb", body: []byte{0x00, 0x01, 0x02, 0x03}},
			},
			wantEntries: "manifest.json,device.json,logs.jsonl,crash/summary.json,crash/tombstone.pb",
		},
		{
			name: "over count",
			entries: append([]testArchiveEntry{
				{name: "manifest.json", body: []byte(`{}`)},
			}, repeatedEntries("logs.jsonl", 16)...),
			wantErr: ErrTooManyEntries,
		},
		{
			name:    "oversize",
			entries: validEntries,
			limits:  BundleLimits{MaxCompressedBytes: 64},
			wantErr: ErrCompressedTooLarge,
		},
		{
			name:    "truncated gzip",
			entries: validEntries,
			mutate: func(raw []byte) []byte {
				return raw[:len(raw)-8]
			},
			wantErr: ErrInvalidBundle,
		},
		{
			name:    "rejects trailing tar data inside gzip stream",
			entries: validEntries,
			mutate: func([]byte) []byte {
				return buildTestArchiveWithPostTarPayload(t, validEntries, []byte("hidden"))
			},
			wantErr: ErrInvalidBundle,
		},
		{
			name:    "accepts tar record padding",
			entries: validEntries,
			mutate: func([]byte) []byte {
				return buildTestArchiveWithPostTarPayload(t, validEntries, make([]byte, 8*1024))
			},
		},
		{
			name:    "rejects nonzero byte inside trailing padding",
			entries: validEntries,
			mutate: func([]byte) []byte {
				padding := make([]byte, 8*1024)
				padding[len(padding)-1] = 1
				return buildTestArchiveWithPostTarPayload(t, validEntries, padding)
			},
			wantErr: ErrInvalidBundle,
		},
		{
			name:    "rejects excessive trailing padding",
			entries: validEntries,
			mutate: func([]byte) []byte {
				return buildTestArchiveWithPostTarPayload(t, validEntries, make([]byte, maxTarTrailingPaddingBytes+1))
			},
			wantErr: ErrInvalidBundle,
		},
		{
			name:    "rejects concatenated gzip member",
			entries: validEntries,
			mutate: func(raw []byte) []byte {
				return append(raw, buildTestGzipMember(t, bytes.Repeat([]byte{0}, 1024))...)
			},
			wantErr: ErrInvalidBundle,
		},
		{
			name: "rejects symlinks",
			entries: []testArchiveEntry{
				{name: "manifest.json", body: []byte(`{}`)},
				{name: "logs.jsonl", typeflag: tar.TypeSymlink},
			},
			wantErr: ErrInvalidBundle,
		},
		{
			name: "rejects padded entry name",
			entries: []testArchiveEntry{
				{name: "manifest.json ", body: []byte(`{}`)},
			},
			wantErr: ErrInvalidBundle,
		},
		{
			name:    "rejects pax extension records",
			entries: validEntries,
			mutate: func([]byte) []byte {
				return buildTestArchivePAX(t)
			},
			wantErr: ErrInvalidBundle,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := buildTestArchive(t, tt.entries)
			if tt.mutate != nil {
				raw = tt.mutate(raw)
			}
			info, err := ValidateBundle(bytes.NewReader(raw), tt.limits)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ValidateBundle error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateBundle error = %v", err)
			}
			if info.CompressedBytes != int64(len(raw)) {
				t.Fatalf("compressed bytes = %d, want %d", info.CompressedBytes, len(raw))
			}
			sum := sha256.Sum256(raw)
			if info.SHA256 != fmtHex(sum[:]) {
				t.Fatalf("sha256 = %s, want %s", info.SHA256, fmtHex(sum[:]))
			}
			wantEntries := tt.wantEntries
			if wantEntries == "" {
				wantEntries = "manifest.json,logs.jsonl"
			}
			if got := strings.Join(info.Entries, ","); got != wantEntries {
				t.Fatalf("entries = %s, want %s", got, wantEntries)
			}
			gzr, err := gzip.NewReader(bytes.NewReader(raw))
			if err != nil {
				t.Fatalf("open gzip for uncompressed size: %v", err)
			}
			wantUncompressed, err := io.Copy(io.Discard, gzr)
			if err != nil {
				t.Fatalf("measure uncompressed size: %v", err)
			}
			if info.UncompressedBytes != wantUncompressed {
				t.Fatalf("uncompressed bytes = %d, want %d", info.UncompressedBytes, wantUncompressed)
			}
		})
	}
}

func repeatedEntries(name string, count int) []testArchiveEntry {
	entries := make([]testArchiveEntry, 0, count)
	for i := 0; i < count; i++ {
		entries = append(entries, testArchiveEntry{name: name, body: []byte("{}\n")})
	}
	return entries
}

func buildTestArchive(t *testing.T, entries []testArchiveEntry) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		typeflag := entry.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		hdr := &tar.Header{
			Name:     entry.name,
			Mode:     0o600,
			Size:     int64(len(entry.body)),
			Typeflag: typeflag,
		}
		if typeflag != tar.TypeReg {
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if hdr.Size > 0 {
			if _, err := tw.Write(entry.body); err != nil {
				t.Fatalf("write body: %v", err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

func buildTestArchiveWithPostTarPayload(t *testing.T, entries []testArchiveEntry, trailing []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		typeflag := entry.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		hdr := &tar.Header{
			Name:     entry.name,
			Mode:     0o600,
			Size:     int64(len(entry.body)),
			Typeflag: typeflag,
		}
		if typeflag != tar.TypeReg {
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if hdr.Size > 0 {
			if _, err := tw.Write(entry.body); err != nil {
				t.Fatalf("write body: %v", err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if _, err := gz.Write(trailing); err != nil {
		t.Fatalf("write trailing gzip payload: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

// buildTestArchivePAX writes an archive whose first entry uses PAX extended
// headers (forced by a custom PAX record). archive/tar consumes those records
// before the entry header surfaces, so validation must reject the format even
// though the visible name is allowlisted.
func buildTestArchivePAX(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte(`{"schema_version":1}`)
	hdr := &tar.Header{
		Name:       "manifest.json",
		Mode:       0o600,
		Size:       int64(len(body)),
		Typeflag:   tar.TypeReg,
		Format:     tar.FormatPAX,
		PAXRecords: map[string]string{"SILO.smuggled": "payload"},
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write pax header: %v", err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatalf("write pax body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

func buildTestGzipMember(t *testing.T, payload []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(payload); err != nil {
		t.Fatalf("write gzip payload: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

func fmtHex(raw []byte) string {
	const table = "0123456789abcdef"
	out := make([]byte, len(raw)*2)
	for i, b := range raw {
		out[i*2] = table[b>>4]
		out[i*2+1] = table[b&0x0f]
	}
	return string(out)
}
