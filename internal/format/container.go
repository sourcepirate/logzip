package format

import (
	"encoding/binary"
	"fmt"
	"io"
	"unicode/utf8"
)

const (
	Magic      = "LOGZ"
	Version    = uint16(1)
	OuterMagic = "ZZST"

	BackendNone = 0
	BackendGzip = 1
	BackendZstd = 2
	BackendLzma = 3

	FlagMultiEntry   = 1 << 0
	FlagHasChecksum  = 1 << 1
	FlagElasticInts  = 1 << 2

	SentinelLoadFailed  = -1
	SentinelMatchFailed = 0

	EncRawStr   = 0
	EncNumeric  = 1
	EncDelta    = 2
	EncDictID   = 3

	ChunkLinesDefault = 100000
	WindowHDefault    = 20
)

type ContainerHeader struct {
	Version    uint16
	Flags      uint16
	BackendID  uint8
	ChunkLines uint32
	WindowH    uint32
	Checksum   uint32
}

type EntryDescriptor struct {
	Name       string
	OrigSize   uint64
	LineCount  uint64
	ChunkCount uint32
	ChunkOffsets []uint64
	FailedOff  uint64
}

type TemplateToken struct {
	Kind   uint8
	Data   string
}

type Template struct {
	Tokens []TemplateToken
}

type Dictionary struct {
	Tag     uint8
	TID     uint32
	Entries []string
}

type HeaderFieldFormat struct {
	StringSubs  uint8
	NumericSubs uint8
	Format      string
	StrLens     []int8
	NumLens     []int8
	Delim       string
}

type HeadFormat struct {
	HeadLength uint32
	IsMulti    bool
	HeadRegex  string
	Fields     []HeaderFieldFormat
}

type GlobalMeta struct {
	HeadFormat  HeadFormat
	Templates   []Template
	Dictionaries []Dictionary
}

type ColumnEncoding struct {
	Enc        uint8
	HasPattern bool
	Pattern    string
	AllPos     []uint8
}

type Chunk struct {
	LineCount     uint32
	EIDs          []int32
	HeaderCols    []ColumnData
	VarGroups     []VarGroup
}

type ColumnData struct {
	Enc     uint8
	Values  interface{}
}

type VarGroup struct {
	TID     uint32
	ColIdx  uint16
	Data    ColumnData
}

type FailedStream struct {
	LoadFailed  [][]byte
	MatchFailed [][]byte
}

type LogzContainer struct {
	Header     ContainerHeader
	Entries    []EntryDescriptor
	GlobalMeta GlobalMeta
	Chunks     []ChunkEntry
	FailedLogs map[string]FailedStream
}

type ChunkEntry struct {
	EntryIndex int32
	ChunkIndex int32
	Chunk      Chunk
	Data       []byte
}

func PutString(buf []byte, s string) int {
	l := len(s)
	binary.LittleEndian.PutUint16(buf, uint16(l))
	copy(buf[2:], s)
	return 2 + l
}

func ReadString(r io.Reader) (string, error) {
	var l16 uint16
	if err := binary.Read(r, binary.LittleEndian, &l16); err != nil {
		return "", err
	}
	buf := make([]byte, l16)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func WriteString(w io.Writer, s string) (int, error) {
	var l16 uint16
	if len(s) > 65535 {
		return 0, fmt.Errorf("string too long: %d", len(s))
	}
	if !utf8.ValidString(s) {
		return 0, fmt.Errorf("string not valid utf-8")
	}
	l16 = uint16(len(s))
	n := 0
	if err := binary.Write(w, binary.LittleEndian, l16); err != nil {
		return n, err
	}
	n += 2
	nn, err := w.Write([]byte(s))
	n += nn
	return n, err
}

func PutUint16(buf []byte, v uint16) {
	binary.LittleEndian.PutUint16(buf, v)
}

func PutUint32(buf []byte, v uint32) {
	binary.LittleEndian.PutUint32(buf, v)
}

func PutUint64(buf []byte, v uint64) {
	binary.LittleEndian.PutUint64(buf, v)
}
