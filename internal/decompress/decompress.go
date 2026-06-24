package decompress

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sathyanarrayanan/logzip/internal/analyzer"
	"github.com/sathyanarrayanan/logzip/internal/cli"
	"github.com/sathyanarrayanan/logzip/internal/format"
)

var ErrNoLogzFiles = errors.New("no .logz files found")

func Run(app *cli.App) error {
	if app.Options.Test {
		return testArchive(app)
	}
	if app.Options.Stdout {
		return decompressToWriter(app.Args[0], os.Stdout, app.Options)
	}

	for _, path := range app.Args {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("cannot access %s: %w", path, err)
		}
		if info.IsDir() {
			if err := decompressDir(path, app.Options); err != nil {
				return fmt.Errorf("decompress dir %s: %w", path, err)
			}
		} else {
			if err := decompressFile(path, app.Options); err != nil {
				return fmt.Errorf("decompress %s: %w", path, err)
			}
		}
	}
	return nil
}

func List(app *cli.App) error {
	for _, path := range app.Args {
		if err := listArchive(path); err != nil {
			return err
		}
	}
	return nil
}

func decompressFile(path string, opts *cli.Options) error {
	if !strings.HasSuffix(path, ".logz") {
		return fmt.Errorf("not a .logz file: %s", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	origName := strings.TrimSuffix(path, ".logz")
	if origName == "" {
		origName = "output.log"
	}

	out, err := os.Create(origName)
	if err != nil {
		return err
	}
	defer out.Close()

	if opts.Verbose {
		fmt.Fprintf(os.Stderr, "decompressing %s -> %s\n", path, origName)
	}

	if err := decompressStream(f, out, opts); err != nil {
		if !opts.Keep {
			os.Remove(origName)
		}
		return err
	}

	if !opts.Keep {
		os.Remove(path)
	}
	return nil
}

func decompressDir(path string, opts *cli.Options) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".logz") {
			p := filepath.Join(path, e.Name())
			if err := decompressFile(p, opts); err != nil {
				return err
			}
		}
	}
	return nil
}

func decompressStream(r io.Reader, w io.Writer, opts *cli.Options) error {
	return decompressImpl(r, w, opts)
}

func decompressImpl(r io.Reader, w io.Writer, opts *cli.Options) error {
	cr, err := format.Open(r)
	if err != nil {
		return fmt.Errorf("open container: %w", err)
	}

	headLen := int(cr.GlobalMeta.HeadFormat.HeadLength)
	if headLen == 0 {
		headLen = 5
	}

	for _, ce := range cr.Chunks {
		if ce.Data == nil || len(ce.Data) == 0 {
			continue
		}

		cd := NewColumnDecoder(ce.Data)

		lineCount := cd.ReadUint32()
		eids := cd.ReadInt32s()
		if len(eids) != int(lineCount) {
			return fmt.Errorf("eid count mismatch: %d vs %d", len(eids), lineCount)
		}

		headerColCount := cd.ReadUint32()
		headers := make([][]string, lineCount)
		for i := range headers {
			headers[i] = make([]string, headerColCount)
		}
		for col := 0; col < int(headerColCount); col++ {
			enc := cd.ReadByte()
			vals := decodeColumnValues(cd, enc, int(lineCount), &cr.GlobalMeta, -1, col)
			if len(vals) == int(lineCount) {
				for i := 0; i < int(lineCount); i++ {
					headers[i][col] = vals[i]
				}
			}
		}

		tmplCount := cd.ReadUint32()
		type varCols struct {
			vals [][]string
		}
		vars := make(map[int]*varCols)
		for ti := 0; ti < int(tmplCount); ti++ {
			tid := int(cd.ReadInt32())
			varCount := cd.ReadUint32()
			var rows [][]string
			for vi := 0; vi < int(varCount); vi++ {
				enc := cd.ReadByte()
				vals := decodeColumnValues(cd, enc, 0, &cr.GlobalMeta, tid, vi)
				if vi == 0 {
					rows = make([][]string, len(vals))
				}
				for j := 0; j < len(rows) && j < len(vals); j++ {
					rows[j] = append(rows[j], vals[j])
				}
			}
			vars[tid] = &varCols{vals: rows}
		}

		tmplByID := make(map[int]*format.Template)
		for i := range cr.GlobalMeta.Templates {
			tmplByID[i+1] = &cr.GlobalMeta.Templates[i]
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
			if i < len(headers) && headers[i] != nil {
				line = strings.Join(headers[i], " ") + " "
			}

			varIdx := 0
			ri := rowIdx[int(eid)]
			tmplVars := vars[int(eid)]
			for _, tok := range tmpl.Tokens {
				if tok.Kind == 0 {
					line += tok.Data + " "
				} else {
					if tmplVars != nil && ri < len(tmplVars.vals) && varIdx < len(tmplVars.vals[ri]) {
						line += tmplVars.vals[ri][varIdx] + " "
					}
					varIdx++
				}
			}

			line = strings.TrimRight(line, " ")
			fmt.Fprintln(w, line)
			rowIdx[int(eid)] = ri + 1
		}
	}

	return nil
}

func decodeColumnValues(cd *ColumnDecoder, enc byte, expectedCount int, globalMeta *format.GlobalMeta, tid, vi int) []string {
	switch enc {
	case format.EncDelta:
		n := int(cd.ReadUint32())
		intVals := cd.ReadInt64s()
		if len(intVals) != n {
			skipStrings(cd, n)
			return nil
		}
		decoded := analyzer.DeltaDecode(intVals)
		strs := make([]string, len(decoded))
		for i, v := range decoded {
			strs[i] = strconv.FormatInt(v, 10)
		}
		return strs

	case format.EncDictID:
		n := int(cd.ReadUint32())
		dict := findDict(globalMeta, tid, vi)
		if dict == nil {
			skipStrings(cd, n)
			return nil
		}
		strs := make([]string, n)
		for i := 0; i < n; i++ {
			idx := int(cd.ReadInt32())
			if idx >= 0 && idx < len(dict.Entries) {
				strs[i] = dict.Entries[idx]
			}
		}
		return strs

	default:
		n := int(cd.ReadUint32())
		if expectedCount > 0 && n != expectedCount {
			skipStrings(cd, n)
			return nil
		}
		strs := make([]string, n)
		for i := 0; i < n; i++ {
			strs[i] = cd.ReadString()
		}
		return strs
	}
}

func findDict(gm *format.GlobalMeta, tid, vi int) *format.Dictionary {
	for i := range gm.Dictionaries {
		d := &gm.Dictionaries[i]
		if d.Tag == 1 && int(d.TID) == tid && tid >= 0 {
			return d
		}
		if d.Tag == 0 && tid < 0 && int(d.TID) == vi {
			return d
		}
	}
	return nil
}

func testArchive(app *cli.App) error {
	for _, path := range app.Args {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		cr, err := format.Open(f)
		f.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		fmt.Fprintf(os.Stderr, "%s: %d entries, %d chunks\n", path, len(cr.Entries), len(cr.Chunks))
	}
	return nil
}

func listArchive(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	cr, err := format.Open(f)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	for _, e := range cr.Entries {
		fmt.Printf("%s  %d lines  %d bytes\n", e.Name, e.LineCount, e.OrigSize)
	}
	return nil
}

func decompressToWriter(path string, w io.Writer, opts *cli.Options) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return decompressStream(f, w, opts)
}

func DecompressBytes(r io.Reader, w io.Writer, opts *cli.Options) error {
	return decompressImpl(r, w, opts)
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

func (cd *ColumnDecoder) ReadByte() byte {
	b := cd.buf[cd.pos]
	cd.pos++
	return b
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
		v, bytes := binary.Varint(cd.buf[cd.pos:])
		vals[i] = v
		cd.pos += bytes
	}
	return vals
}

func skipStrings(cd *ColumnDecoder, n int) {
	for i := 0; i < n; i++ {
		cd.ReadString()
	}
}

