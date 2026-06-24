package compress

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/sathyanarrayanan/logzip/internal/cli"
	"github.com/sathyanarrayanan/logzip/internal/decompress"
)

func BenchmarkCompressSmall(b *testing.B) {
	input := "192.168.1.1 - - [10/Oct/2000:13:55:36 -0700] GET /index.html HTTP/1.0 200 2326\n" +
		"192.168.1.2 - - [10/Oct/2000:13:55:37 -0700] GET /about.html HTTP/1.0 200 1234\n"
	opts := &cli.Options{
		Backend:    "none",
		Level:      1,
		ChunkLines: 100000,
		Window:     20,
		Verify:     false,
	}
	for i := 0; i < b.N; i++ {
		in := bytes.NewBufferString(input)
		var out bytes.Buffer
		if err := compressImpl(in, &out, opts, "test.log"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompressRepeat(b *testing.B) {
	var lines []string
	for i := 0; i < 1000; i++ {
		lines = append(lines, fmt.Sprintf("192.168.1.%d - - [10/Oct/2000:13:55:%02d -0700] GET /page%d.html HTTP/1.0 200 %d", i%10, i%60, i%50, 100+i*10))
	}
	input := strings.Join(lines, "\n")
	opts := &cli.Options{
		Backend:    "zstd",
		Level:      1,
		ChunkLines: 100000,
		Window:     20,
		Verify:     false,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		in := bytes.NewBufferString(input)
		var out bytes.Buffer
		if err := compressImpl(in, &out, opts, "test.log"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRoundTripComposite(b *testing.B) {
	var lines []string
	for i := 0; i < 500; i++ {
		lines = append(lines, fmt.Sprintf("192.168.1.%d - - [10/Oct/2000:13:55:%02d -0700] \"GET /page%d.html HTTP/1.0\" 200 %d", i%10, i%60, i%50, 100+i*10))
	}
	input := strings.Join(lines, "\n")
	opts := &cli.Options{
		Backend:    "zstd",
		Level:      3,
		ChunkLines: 100000,
		Window:     20,
		Verify:     false,
	}

	in := bytes.NewBufferString(input)
	var compressed bytes.Buffer
	if err := compressImpl(in, &compressed, opts, "test.log"); err != nil {
		b.Fatal(err)
	}
	compressedLen := compressed.Len()
	ratio := float64(compressedLen) / float64(len(input)) * 100
	b.Logf("input: %d bytes, compressed: %d bytes (%.1f%% ratio)", len(input), compressedLen, ratio)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var decompressed bytes.Buffer
		if err := decompress.DecompressBytes(bytes.NewReader(compressed.Bytes()), &decompressed, opts); err != nil {
			b.Fatal(err)
		}
	}
}

func TestCompressionRatioLog(t *testing.T) {
	var lines []string
	for i := 0; i < 10000; i++ {
		lines = append(lines, fmt.Sprintf("192.168.1.%d - - [10/Oct/2000:13:55:%02d -0700] \"%s /page%d.html HTTP/1.0\" %d %d",
			i%20, i%60, []string{"GET", "POST", "PUT"}[i%3], i%100, []int{200, 404, 500, 304}[i%4], 100+(i*13)%9000))
	}
	input := strings.Join(lines, "\n")

	for _, backend := range []string{"none", "gzip", "zstd"} {
		t.Run("backend_"+backend, func(t *testing.T) {
			opts := &cli.Options{
				Backend:    backend,
				Level:      1,
				ChunkLines: 100000,
				Window:     20,
				Verify:     false,
			}
			in := bytes.NewBufferString(input)
			var compressed bytes.Buffer
			if err := compressImpl(in, &compressed, opts, "test.log"); err != nil {
				t.Fatal(err)
			}
			ratio := float64(compressed.Len()) / float64(len(input)) * 100
			t.Logf("ratio: %.1f%% (%d -> %d bytes)", ratio, len(input), compressed.Len())

			var decompressed bytes.Buffer
			if err := decompress.DecompressBytes(bytes.NewReader(compressed.Bytes()), &decompressed, opts); err != nil {
				t.Fatal(err)
			}
			got := strings.TrimSpace(decompressed.String())
			want := strings.TrimSpace(input)
			if got != want {
				t.Fatal("round-trip mismatch")
			}
		})
	}
}
