package format

import (
	"bytes"
	"testing"
)

func TestContainerWriteRead(t *testing.T) {
	var buf bytes.Buffer

	cw := NewContainerWriter(&buf, &ContainerOptions{
		Backend:    "none",
		Level:      6,
		ChunkLines: 100000,
		Window:     20,
		Verify:     false,
	})

	gm := &GlobalMeta{
		HeadFormat: HeadFormat{
			HeadLength: 5,
		},
		Templates: []Template{
			{Tokens: []TemplateToken{{Kind: 0, Data: "GET"}, {Kind: 1, Data: ""}}},
		},
	}
	cw.SetGlobalMeta(gm)
	cw.AddEntry(EntryDescriptor{
		Name:       "test.log",
		OrigSize:   100,
		LineCount:  10,
		ChunkCount: 1,
		ChunkOffsets: []uint64{0},
	})
	cw.AddChunk(0, []byte("hello chunk data"))

	if err := cw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	t.Logf("wrote %d bytes: %x", buf.Len(), buf.Bytes()[:20])

	if buf.Len() < 10 {
		t.Fatal("too small")
	}

	header := buf.Bytes()[:4]
	if string(header) != Magic {
		t.Fatalf("bad magic: got %x, want LOGZ", header)
	}

	cr, err := Open(&buf)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if len(cr.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(cr.Entries))
	}
	if len(cr.Chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(cr.Chunks))
	}
	if string(cr.Chunks[0].Data) != "hello chunk data" {
		t.Fatalf("chunk data mismatch: %q", string(cr.Chunks[0].Data))
	}
}

func TestContainerZstdRoundTrip(t *testing.T) {
	var buf bytes.Buffer

	cw := NewContainerWriter(&buf, &ContainerOptions{
		Backend:    "zstd",
		Level:      1,
		ChunkLines: 100000,
		Window:     20,
		Verify:     false,
	})

	gm := &GlobalMeta{
		HeadFormat: HeadFormat{HeadLength: 3},
	}
	cw.SetGlobalMeta(gm)
	cw.AddEntry(EntryDescriptor{
		Name:       "test.log",
		OrigSize:   50,
		LineCount:  5,
		ChunkCount: 1,
		ChunkOffsets: []uint64{0},
	})
	cw.AddChunk(0, []byte("zstd chunk test"))

	if err := cw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	t.Logf("zstd wrote %d bytes", buf.Len())

	outer := buf.Bytes()[:4]
	if string(outer) != OuterMagic {
		t.Fatalf("bad outer magic: got %x, want ZZST", outer)
	}

	cr, err := Open(&buf)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if string(cr.Chunks[0].Data) != "zstd chunk test" {
		t.Fatalf("chunk data mismatch: %q", string(cr.Chunks[0].Data))
	}
}

func TestWriteReadLogData(t *testing.T) {
	var buf bytes.Buffer

	cw := NewContainerWriter(&buf, &ContainerOptions{
		Backend:    "none",
		ChunkLines: 1000,
		Window:     20,
	})

	logData := []byte("this is some test log data for the container format round trip test")

	gm := &GlobalMeta{
		HeadFormat: HeadFormat{HeadLength: 2},
	}
	cw.SetGlobalMeta(gm)
	cw.AddEntry(EntryDescriptor{
		Name:       "data.log",
		OrigSize:   uint64(len(logData)),
		LineCount:  1,
		ChunkCount: 1,
		ChunkOffsets: []uint64{0},
	})
	cw.AddChunk(0, logData)

	if err := cw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	cr, err := Open(&buf)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if len(cr.Chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(cr.Chunks))
	}
	if string(cr.Chunks[0].Data) != string(logData) {
		t.Fatalf("data mismatch:\ngot:  %q\nwant: %q", string(cr.Chunks[0].Data), string(logData))
	}

	if len(cr.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(cr.Entries))
	}
	if cr.Entries[0].Name != "data.log" {
		t.Fatalf("entry name: %q", cr.Entries[0].Name)
	}
}

func TestContainerSimple(t *testing.T) {
	var buf bytes.Buffer

	cw := NewContainerWriter(&buf, &ContainerOptions{
		Backend: "none",
	})
	cw.SetGlobalMeta(&GlobalMeta{
		HeadFormat: HeadFormat{HeadLength: 1},
	})
	cw.AddEntry(EntryDescriptor{
		Name:       "a.log",
		OrigSize:   100,
		LineCount:  10,
		ChunkCount: 0,
	})
	if err := cw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	dump := buf.Bytes()
	t.Logf("wrote %d bytes", len(dump))
	if len(dump) < 8 {
		t.Fatalf("too small: %d", len(dump))
	}
	t.Logf("first 16 bytes: %x", dump[:16])
	if string(dump[:4]) != Magic {
		t.Fatalf("bad magic, got: %x", dump[:4])
	}

	t.Logf("string dump: %q", string(dump[:40]))

	cr, err := Open(&buf)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if len(cr.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(cr.Entries))
	}
}
