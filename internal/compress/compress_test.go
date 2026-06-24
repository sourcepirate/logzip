package compress

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sathyanarrayanan/logzip/internal/cli"
	"github.com/sathyanarrayanan/logzip/internal/decompress"
)

func TestCompressDecompressRoundTrip(t *testing.T) {
	input := `192.168.1.1 - - [10/Oct/2000:13:55:36 -0700] GET /index.html HTTP/1.0 200 2326
192.168.1.2 - - [10/Oct/2000:13:55:37 -0700] GET /about.html HTTP/1.0 200 1234
192.168.1.3 - - [10/Oct/2000:13:55:38 -0700] POST /api/login HTTP/1.0 401 512
192.168.1.4 - - [10/Oct/2000:13:55:39 -0700] GET /index.html HTTP/1.0 200 2326
192.168.1.5 - - [10/Oct/2000:13:55:40 -0700] GET /contact.html HTTP/1.0 404 256
`

	in := strings.NewReader(input)
	var compressed bytes.Buffer

	opts := &cli.Options{
		Backend:    "none",
		Level:      6,
		ChunkLines: 100000,
		Window:     20,
		Verify:     false,
	}
	err := compressImpl(in, &compressed, opts, "test.log")
	if err != nil {
		t.Fatalf("compress: %v", err)
	}

	if compressed.Len() == 0 {
		t.Fatal("compressed output is empty")
	}
	t.Logf("compressed %d bytes into %d bytes (ratio: %.2f%%)", len(input), compressed.Len(), float64(compressed.Len())/float64(len(input))*100)

	var decompressed bytes.Buffer
	err = decompress.DecompressBytes(&compressed, &decompressed, opts)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}

	got := strings.TrimSpace(decompressed.String())
	want := strings.TrimSpace(input)
	if got != want {
		t.Fatalf("round trip mismatch:\ngot len=%d:\n%s\n\nwant len=%d:\n%s", len(got), got, len(want), want)
	}
	t.Logf("round trip OK: %d bytes -> %d -> %d bytes", len(input), compressed.Len(), decompressed.Len())
}

func TestCompressLargeLog(t *testing.T) {
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, fmt.Sprintf("192.168.1.%d - - [10/Oct/2000:13:55:%02d -0700] GET /page%d.html HTTP/1.0 200 %d", i%10, i%60, i%50, 100+i*10))
	}
	input := strings.Join(lines, "\n")

	in := strings.NewReader(input)
	var compressed bytes.Buffer

	opts := &cli.Options{
		Backend:    "none",
		Level:      6,
		ChunkLines: 100000,
		Window:     20,
		Verify:     false,
	}
	err := compressImpl(in, &compressed, opts, "test.log")
	if err != nil {
		t.Fatalf("compress: %v", err)
	}

	var decompressed bytes.Buffer
	err = decompress.DecompressBytes(&compressed, &decompressed, opts)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}

	got := strings.TrimSpace(decompressed.String())
	want := strings.TrimSpace(input)
	if got != want {
		t.Fatalf("round trip mismatch:\ngot len=%d\nwant len=%d", len(got), len(want))
	}
	t.Logf("large log round trip OK: %d bytes -> %d -> %d bytes (ratio: %.2f%%)",
		len(input), compressed.Len(), decompressed.Len(), float64(compressed.Len())/float64(len(input))*100)
}

func TestCompressWithZstd(t *testing.T) {
	input := `192.168.1.1 - - [10/Oct/2000:13:55:36 -0700] GET /index.html HTTP/1.0 200 2326
192.168.1.2 - - [10/Oct/2000:13:55:37 -0700] GET /about.html HTTP/1.0 200 1234
192.168.1.3 - - [10/Oct/2000:13:55:38 -0700] POST /api/login HTTP/1.0 401 512
192.168.1.4 - - [10/Oct/2000:13:55:39 -0700] GET /index.html HTTP/1.0 200 2326
192.168.1.5 - - [10/Oct/2000:13:55:40 -0700] GET /contact.html HTTP/1.0 404 256
192.168.1.6 - - [10/Oct/2000:13:55:41 -0700] GET /index.html HTTP/1.0 200 2326
`

	in := strings.NewReader(input)
	var compressed bytes.Buffer

	opts := &cli.Options{
		Backend:    "zstd",
		Level:      1,
		ChunkLines: 100000,
		Window:     20,
		Verify:     false,
	}
	err := compressImpl(in, &compressed, opts, "test.log")
	if err != nil {
		t.Fatalf("compress: %v", err)
	}

	t.Logf("compressed %d bytes into %d bytes (ratio: %.2f%%)", len(input), compressed.Len(), float64(compressed.Len())/float64(len(input))*100)

	var decompressed bytes.Buffer
	err = decompress.DecompressBytes(&compressed, &decompressed, opts)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}

	got := strings.TrimSpace(decompressed.String())
	want := strings.TrimSpace(input)
	if got != want {
		t.Fatalf("round trip mismatch:\ngot: %q\nwant: %q", got, want)
	}
}

func TestCompressLogFile(t *testing.T) {
	input := `192.168.1.1 - - [10/Oct/2000:13:55:36 -0700] "GET /index.html HTTP/1.0" 200 2326
192.168.1.2 - - [10/Oct/2000:13:55:37 -0700] "GET /about.html HTTP/1.0" 200 1234
`
	in := strings.NewReader(input)
	var compressed bytes.Buffer

	opts := &cli.Options{
		Backend:    "none",
		Level:      6,
		ChunkLines: 100000,
		Window:     20,
		Verify:     false,
	}
	err := compressImpl(in, &compressed, opts, "access.log")
	if err != nil {
		t.Fatalf("compress: %v", err)
	}

	var decompressed bytes.Buffer
	err = decompress.DecompressBytes(&compressed, &decompressed, opts)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}

	got := strings.TrimSpace(decompressed.String())
	want := strings.TrimSpace(input)
	if got != want {
		t.Fatalf("round trip mismatch:\ngot: %q\nwant: %q", got, want)
	}
}

func TestCompressFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	logContent := "192.168.1.1 - - [10/Oct/2000:13:55:36 -0700] GET /index.html HTTP/1.0 200 2326\n" +
		"192.168.1.2 - - [10/Oct/2000:13:55:37 -0700] GET /about.html HTTP/1.0 200 1234\n" +
		"192.168.1.3 - - [10/Oct/2000:13:55:38 -0700] POST /api/login HTTP/1.0 401 512\n"
	if err := os.WriteFile(logPath, []byte(logContent), 0644); err != nil {
		t.Fatal(err)
	}

	var compressed bytes.Buffer
	f, err := os.Open(logPath)
	if err != nil {
		t.Fatal(err)
	}
	opts := &cli.Options{
		Backend:    "none",
		Level:      6,
		ChunkLines: 100000,
		Window:     20,
		Verify:     false,
	}
	if err := compressImpl(f, &compressed, opts, "test.log"); err != nil {
		f.Close()
		t.Fatalf("compress: %v", err)
	}
	f.Close()

	var decompressed bytes.Buffer
	if err := decompress.DecompressBytes(&compressed, &decompressed, opts); err != nil {
		t.Fatalf("decompress: %v", err)
	}

	got := strings.TrimSpace(decompressed.String())
	want := strings.TrimSpace(logContent)
	if got != want {
		t.Fatalf("round trip mismatch:\ngot:  %q\nwant: %q", got, want)
	}
}
