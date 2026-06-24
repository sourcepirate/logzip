package compress

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/sathyanarrayanan/logzip/internal/analyzer"
	"github.com/sathyanarrayanan/logzip/internal/format"
	"github.com/sathyanarrayanan/logzip/internal/parser"
)

type LogMeta struct {
	Lines      []parser.LogLine
	EIDs       []int32
	Headers    [][]string
	Variables  map[int][][]string
	Templates  []*parser.Template
	TmplMap    map[int]*parser.Template
}

func ParseLogs(r io.Reader, headLen int, drainDepth int, st float64) (*parser.HeadFormat, *LogMeta, error) {
	var allLines []string
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("scan: %w", err)
	}

	if len(allLines) == 0 {
		return nil, &LogMeta{}, nil
	}

	sampleSize := 1000
	if len(allLines) < sampleSize {
		sampleSize = len(allLines)
	}
	sample := allLines[:sampleSize]

	hf := parser.InferHeaderSchema(sample, headLen)

	dp := parser.NewDrainParser(drainDepth, 100, st)
	for _, line := range sample {
		_, content := parser.SplitHeader(line, headLen)
		if content != "" {
			dp.AddLine(content)
		}
	}

	meta := &LogMeta{
		EIDs:      make([]int32, len(allLines)),
		Headers:   make([][]string, len(allLines)),
		Variables: make(map[int][][]string),
		TmplMap:   make(map[int]*parser.Template),
	}

	for _, tmpl := range dp.Templates {
		meta.Templates = append(meta.Templates, tmpl)
		meta.TmplMap[tmpl.ID] = tmpl
	}

	for i, line := range allLines {
		header, content := parser.SplitHeader(line, headLen)
		if len(header) < headLen {
			meta.EIDs[i] = format.SentinelLoadFailed
			meta.Headers[i] = nil
			continue
		}

		tmpl, vars := dp.MatchWithVars(content)
		if tmpl == nil {
			meta.EIDs[i] = format.SentinelMatchFailed
			meta.Headers[i] = header
			continue
		}

		meta.EIDs[i] = int32(tmpl.ID)
		meta.Headers[i] = header
		if _, ok := meta.Variables[tmpl.ID]; !ok {
			meta.Variables[tmpl.ID] = nil
		}
		meta.Variables[tmpl.ID] = append(meta.Variables[tmpl.ID], vars)
	}

	return hf, meta, nil
}

type ColumnEncoder struct {
	buf *bytes.Buffer
}

func NewColumnEncoder() *ColumnEncoder {
	return &ColumnEncoder{buf: new(bytes.Buffer)}
}

func (ce *ColumnEncoder) WriteInt32(v int32) {
	b := make([]byte, binary.MaxVarintLen32)
	n := binary.PutVarint(b, int64(v))
	ce.buf.Write(b[:n])
}

func (ce *ColumnEncoder) WriteUint32(v uint32) {
	b := make([]byte, binary.MaxVarintLen32)
	n := binary.PutUvarint(b, uint64(v))
	ce.buf.Write(b[:n])
}

func (ce *ColumnEncoder) WriteString(s string) {
	ce.WriteUint32(uint32(len(s)))
	ce.buf.WriteString(s)
}

func (ce *ColumnEncoder) WriteBytes(b []byte) {
	ce.WriteUint32(uint32(len(b)))
	ce.buf.Write(b)
}

func (ce *ColumnEncoder) WriteByte(b byte) {
	ce.buf.WriteByte(b)
}

func (ce *ColumnEncoder) WriteInt32s(vals []int32) {
	ce.WriteUint32(uint32(len(vals)))
	for _, v := range vals {
		ce.WriteInt32(v)
	}
}

func (ce *ColumnEncoder) WriteInt64s(vals []int64) {
	ce.WriteUint32(uint32(len(vals)))
	for _, v := range vals {
		b := make([]byte, binary.MaxVarintLen64)
		n := binary.PutVarint(b, v)
		ce.buf.Write(b[:n])
	}
}

func (ce *ColumnEncoder) Bytes() []byte {
	return ce.buf.Bytes()
}

type ColumnDecoder struct {
	buf []byte
	pos int
}

func NewColumnDecoder(data []byte) *ColumnDecoder {
	return &ColumnDecoder{buf: data}
}

func (cd *ColumnDecoder) ReadInt32() int32 {
	v, n := binary.Varint(cd.buf[cd.pos:])
	cd.pos += n
	return int32(v)
}

func (cd *ColumnDecoder) ReadUint32() uint32 {
	v, n := binary.Uvarint(cd.buf[cd.pos:])
	cd.pos += n
	return uint32(v)
}

func (cd *ColumnDecoder) ReadString() string {
	l := cd.ReadUint32()
	s := string(cd.buf[cd.pos : cd.pos+int(l)])
	cd.pos += int(l)
	return s
}

func (cd *ColumnDecoder) ReadBytes() []byte {
	l := cd.ReadUint32()
	b := cd.buf[cd.pos : cd.pos+int(l)]
	cd.pos += int(l)
	return b
}

func (cd *ColumnDecoder) Remaining() int {
	return len(cd.buf) - cd.pos
}

func (cd *ColumnDecoder) ReadInt32s() []int32 {
	n := cd.ReadUint32()
	vals := make([]int32, n)
	for i := range vals {
		vals[i] = cd.ReadInt32()
	}
	return vals
}

func (cd *ColumnDecoder) ReadInt64s() []int64 {
	n := cd.ReadUint32()
	vals := make([]int64, n)
	for i := range vals {
		vals[i], _ = binary.Varint(cd.buf[cd.pos:])
		cd.pos += binary.Size(vals[i])
	}
	return vals
}

func encodeChunk(meta *LogMeta, eidStart, eidEnd int, headLen int) []byte {
	ce := NewColumnEncoder()

	lineCount := eidEnd - eidStart
	ce.WriteUint32(uint32(lineCount))

	eids := meta.EIDs[eidStart:eidEnd]
	ce.WriteInt32s(eids)

	headerColCount := headLen
	ce.WriteUint32(uint32(headerColCount))
	for col := 0; col < headerColCount; col++ {
		ce.WriteByte(format.EncRawStr)
		ce.WriteUint32(uint32(lineCount))
		for i := eidStart; i < eidEnd; i++ {
			if meta.Headers[i] != nil && col < len(meta.Headers[i]) {
				ce.WriteString(meta.Headers[i][col])
			} else {
				ce.WriteString("")
			}
		}
	}

	ce.WriteUint32(uint32(len(meta.Templates)))
	for _, tmpl := range meta.Templates {
		tmplLines, ok := meta.Variables[tmpl.ID]
		if !ok {
			tmplLines = nil
		}
		ce.WriteInt32(int32(tmpl.ID))
		varCount := len(tmpl.VarTokens())
		ce.WriteUint32(uint32(varCount))
		for vi := 0; vi < varCount; vi++ {
			ce.WriteByte(format.EncRawStr)
			ce.WriteUint32(uint32(len(tmplLines)))
			for _, line := range tmplLines {
				if vi < len(line) {
					ce.WriteString(line[vi])
				} else {
					ce.WriteString("")
				}
			}
		}
	}

	return ce.Bytes()
}

func encodeChunkWithAnalyzer(meta *LogMeta, eidStart, eidEnd int, headLen int, relations *analyzer.Relations) []byte {
	ce := NewColumnEncoder()

	lineCount := eidEnd - eidStart
	ce.WriteUint32(uint32(lineCount))

	eids := meta.EIDs[eidStart:eidEnd]
	ce.WriteInt32s(eids)

	headerColCount := headLen
	ce.WriteUint32(uint32(headerColCount))
	for col := 0; col < headerColCount; col++ {
		dynCol := make([]string, 0, lineCount)
		for i := eidStart; i < eidEnd; i++ {
			if meta.Headers[i] != nil && col < len(meta.Headers[i]) {
				dynCol = append(dynCol, meta.Headers[i][col])
			} else {
				dynCol = append(dynCol, "")
			}
		}
		ce.WriteByte(format.EncRawStr)
		ce.WriteUint32(uint32(len(dynCol)))
		for _, v := range dynCol {
			ce.WriteString(v)
		}
	}

	ce.WriteUint32(uint32(len(meta.Templates)))
	for _, tmpl := range meta.Templates {
		tmplLines, ok := meta.Variables[tmpl.ID]
		if !ok {
			tmplLines = nil
		}
		ce.WriteInt32(int32(tmpl.ID))
		varCount := len(tmpl.VarTokens())
		ce.WriteUint32(uint32(varCount))
		for vi := 0; vi < varCount; vi++ {
			col := make([]string, 0, len(tmplLines))
			for _, line := range tmplLines {
				if vi < len(line) {
					col = append(col, line[vi])
				} else {
					col = append(col, "")
				}
			}
			writeColumnWithAnalysis(ce, col, relations, tmpl.ID, vi)
		}
	}

	return ce.Bytes()
}

func writeColumnWithAnalysis(ce *ColumnEncoder, col []string, relations *analyzer.Relations, tid, vi int) {
	if len(col) == 0 {
		ce.WriteByte(format.EncRawStr)
		ce.WriteUint32(0)
		return
	}

	allNumeric := true
	for _, v := range col {
		if v == "" || !analyzer.IsNumeric(v) {
			allNumeric = false
			break
		}
	}

	if allNumeric && len(col) > 2 {
		intVals := make([]int64, len(col))
		for i, v := range col {
			f, _ := strconv.ParseFloat(v, 64)
			intVals[i] = int64(f)
		}

		if analyzer.ShouldDeltaEncode(intVals) {
			deltas := analyzer.DeltaEncode(intVals)
			ce.WriteByte(format.EncDelta)
			ce.WriteUint32(uint32(len(deltas)))
			ce.WriteInt64s(deltas)
			return
		}
	}

	if relations != nil {
		if info, ok := relations.VarDict[tid]; ok {
			if d, ok := info[vi]; ok {
				ce.WriteByte(format.EncDictID)
				ce.WriteUint32(uint32(len(col)))
				for _, v := range col {
					idx := int32(d.Mapping[v])
					ce.WriteInt32(idx)
				}
				return
			}
		}
	}

	ce.WriteByte(format.EncRawStr)
	ce.WriteUint32(uint32(len(col)))
	for _, v := range col {
		ce.WriteString(v)
	}
}

func decodeChunk(data []byte, globalMeta *format.GlobalMeta, headLen int) (eids []int32, headers [][]string, vars map[int][][]string, err error) {
	cd := NewColumnDecoder(data)

	lineCount := cd.ReadUint32()
	eids = cd.ReadInt32s()
	if len(eids) != int(lineCount) {
		return nil, nil, nil, fmt.Errorf("eid count mismatch: %d vs %d", len(eids), lineCount)
	}

	headerColCount := cd.ReadUint32()
	headers = make([][]string, lineCount)
	for i := range headers {
		headers[i] = make([]string, headerColCount)
	}
	for col := 0; col < int(headerColCount); col++ {
		enc := cd.ReadByte()
		_ = enc
		valCount := cd.ReadUint32()
		if int(valCount) != int(lineCount) {
			cd.SkipStrings(int(valCount))
			continue
		}
		for i := 0; i < int(lineCount); i++ {
			headers[i][col] = cd.ReadString()
		}
	}

	tmplCount := cd.ReadUint32()
	vars = make(map[int][][]string)
	for ti := 0; ti < int(tmplCount); ti++ {
		tid := int(cd.ReadInt32())
		varCount := cd.ReadUint32()
		linesCount := uint32(0)
		var tmplLines [][]string
		for vi := 0; vi < int(varCount); vi++ {
			enc := cd.ReadByte()
			_ = enc
			if vi == 0 {
				linesCount = cd.ReadUint32()
			} else {
				cd.ReadUint32()
			}
			col := make([]string, linesCount)
			for j := 0; j < int(linesCount); j++ {
				col[j] = cd.ReadString()
			}
			if vi == 0 {
				tmplLines = make([][]string, linesCount)
			}
			for j := 0; j < int(linesCount); j++ {
				tmplLines[j] = append(tmplLines[j], col[j])
			}
		}
		vars[tid] = tmplLines
	}

	return eids, headers, vars, nil
}

func (cd *ColumnDecoder) ReadByte() byte {
	b := cd.buf[cd.pos]
	cd.pos++
	return b
}

func (cd *ColumnDecoder) SkipStrings(n int) {
	for i := 0; i < n; i++ {
		cd.ReadString()
	}
}

func decompressChunk(data []byte, globalMeta *format.GlobalMeta, headLen int, w io.Writer) error {
	eids, headers, vars, err := decodeChunk(data, globalMeta, headLen)
	if err != nil {
		return fmt.Errorf("decode chunk: %w", err)
	}

	tmplByID := make(map[int]*format.Template)
	for i := range globalMeta.Templates {
		tmplByID[i+1] = &globalMeta.Templates[i]
	}

	rowIdx := make(map[int]int)
	for i, eid := range eids {
		if eid <= 0 {
			continue
		}
		tmpl, ok := tmplByID[int(eid)]
		if !ok {
			continue
		}

		var line string
		if headers[i] != nil {
			line = strings.Join(headers[i], " ") + " "
		}

		ri := rowIdx[int(eid)]
		tmplVars, hasVars := vars[int(eid)]
		varIdx := 0
		for _, tok := range tmpl.Tokens {
			if tok.Kind == 0 {
				line += tok.Data + " "
			} else {
				if hasVars && ri < len(tmplVars) && varIdx < len(tmplVars[ri]) {
					line += tmplVars[ri][varIdx] + " "
				}
				varIdx++
			}
		}

		line = strings.TrimRight(line, " ")
		fmt.Fprintln(w, line)
		rowIdx[int(eid)] = ri + 1
	}

	return nil
}
