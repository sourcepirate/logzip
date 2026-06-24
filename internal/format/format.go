package format

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash"
	"hash/crc32"
	"io"

	"github.com/sathyanarrayanan/logzip/internal/backend"
)

type ContainerWriter struct {
	w           io.Writer
	opts        *ContainerOptions
	header      ContainerHeader
	entries     []EntryDescriptor
	globalMeta  *GlobalMeta
	chunkData   []chunkBuffer
	failedData  map[string]FailedStream
	hash        hash.Hash32
	payloadBuf  bytes.Buffer
}

type ContainerOptions struct {
	Backend    string
	Level      int
	ChunkLines int
	Window     int
	Verify     bool
}

type chunkBuffer struct {
	entryIndex int32
	data       []byte
}

func NewContainerWriter(w io.Writer, opts *ContainerOptions) *ContainerWriter {
	return &ContainerWriter{
		w:          w,
		opts:       opts,
		chunkData:  nil,
		failedData: make(map[string]FailedStream),
	}
}

func (cw *ContainerWriter) SetGlobalMeta(meta *GlobalMeta) {
	cw.globalMeta = meta
}

func (cw *ContainerWriter) AddEntry(desc EntryDescriptor) {
	cw.entries = append(cw.entries, desc)
}

func (cw *ContainerWriter) AddChunk(entryIndex int32, data []byte) {
	cw.chunkData = append(cw.chunkData, chunkBuffer{entryIndex: entryIndex, data: data})
}

func (cw *ContainerWriter) SetFailedLogs(entryName string, fs FailedStream) {
	cw.failedData[entryName] = fs
}

func (cw *ContainerWriter) Close() error {
	backendID := uint8(0)
	switch cw.opts.Backend {
	case "gzip":
		backendID = BackendGzip
	case "zstd":
		backendID = BackendZstd
	case "lzma":
		backendID = BackendLzma
	}

	flags := uint16(0)
	if len(cw.entries) > 1 {
		flags |= FlagMultiEntry
	}

	cw.header = ContainerHeader{
		Version:    Version,
		Flags:      flags,
		BackendID:  backendID,
		ChunkLines: uint32(cw.opts.ChunkLines),
		WindowH:    uint32(cw.opts.Window),
	}

	if cw.opts.Verify {
		cw.hash = crc32.NewIEEE()
	}

	cw.payloadBuf.Reset()
	if err := cw.writeContainer(&cw.payloadBuf); err != nil {
		return err
	}
	payload := cw.payloadBuf.Bytes()

	if backendID != BackendNone {
		comp, err := backend.Get(cw.opts.Backend)
		if err != nil {
			return err
		}

		var compressedBuf bytes.Buffer
		compWriter, err := comp.NewWriter(&compressedBuf, cw.opts.Level)
		if err != nil {
			return err
		}
		if _, err := compWriter.Write(payload); err != nil {
			compWriter.Close()
			return err
		}
		if err := compWriter.Close(); err != nil {
			return err
		}

		compressedPayload := compressedBuf.Bytes()
		if _, err := cw.w.Write([]byte(OuterMagic)); err != nil {
			return err
		}
		if err := binary.Write(cw.w, binary.LittleEndian, backendID); err != nil {
			return err
		}
		payloadLen := uint64(len(compressedPayload))
		if err := binary.Write(cw.w, binary.LittleEndian, payloadLen); err != nil {
			return err
		}
		if _, err := cw.w.Write(compressedPayload); err != nil {
			return err
		}
	} else {
		if _, err := cw.w.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

func (cw *ContainerWriter) writeContainer(buf io.Writer) error {
	if err := cw.writeHeader(buf); err != nil {
		return err
	}
	if err := cw.writeEntryTable(buf); err != nil {
		return err
	}
	if err := cw.writeGlobalMeta(buf); err != nil {
		return err
	}
	if err := cw.writeChunks(buf); err != nil {
		return err
	}
	if err := cw.writeFailedStreams(buf); err != nil {
		return err
	}
	return nil
}

func (cw *ContainerWriter) writeHeader(buf io.Writer) error {
	buf.Write([]byte(Magic))
	binary.Write(buf, binary.LittleEndian, cw.header.Version)
	binary.Write(buf, binary.LittleEndian, cw.header.Flags)
	binary.Write(buf, binary.LittleEndian, cw.header.BackendID)
	binary.Write(buf, binary.LittleEndian, cw.header.ChunkLines)
	binary.Write(buf, binary.LittleEndian, cw.header.WindowH)
	if cw.header.Flags&FlagHasChecksum != 0 {
		binary.Write(buf, binary.LittleEndian, cw.header.Checksum)
	}
	return nil
}

func (cw *ContainerWriter) writeEntryTable(buf io.Writer) error {
	binary.Write(buf, binary.LittleEndian, uint32(len(cw.entries)))
	for _, e := range cw.entries {
		WriteString(buf, e.Name)
		binary.Write(buf, binary.LittleEndian, e.OrigSize)
		binary.Write(buf, binary.LittleEndian, e.LineCount)
		binary.Write(buf, binary.LittleEndian, e.ChunkCount)
		for _, off := range e.ChunkOffsets {
			binary.Write(buf, binary.LittleEndian, off)
		}
		binary.Write(buf, binary.LittleEndian, e.FailedOff)
	}
	return nil
}

func (cw *ContainerWriter) writeGlobalMeta(buf io.Writer) error {
	hf := &cw.globalMeta.HeadFormat
	binary.Write(buf, binary.LittleEndian, hf.HeadLength)
	isMulti := uint8(0)
	if hf.IsMulti {
		isMulti = 1
	}
	binary.Write(buf, binary.LittleEndian, isMulti)
	WriteString(buf, hf.HeadRegex)
	binary.Write(buf, binary.LittleEndian, uint32(len(hf.Fields)))
	for _, f := range hf.Fields {
		binary.Write(buf, binary.LittleEndian, f.StringSubs)
		binary.Write(buf, binary.LittleEndian, f.NumericSubs)
		WriteString(buf, f.Format)
		binary.Write(buf, binary.LittleEndian, uint8(len(f.StrLens)))
		for _, sl := range f.StrLens {
			binary.Write(buf, binary.LittleEndian, sl)
		}
		binary.Write(buf, binary.LittleEndian, uint8(len(f.NumLens)))
		for _, nl := range f.NumLens {
			binary.Write(buf, binary.LittleEndian, nl)
		}
		WriteString(buf, f.Delim)
	}

	binary.Write(buf, binary.LittleEndian, uint32(len(cw.globalMeta.Templates)))
	for _, tmpl := range cw.globalMeta.Templates {
		binary.Write(buf, binary.LittleEndian, uint16(len(tmpl.Tokens)))
		for _, tok := range tmpl.Tokens {
			binary.Write(buf, binary.LittleEndian, tok.Kind)
			WriteString(buf, tok.Data)
		}
	}

	binary.Write(buf, binary.LittleEndian, uint32(len(cw.globalMeta.Dictionaries)))
	for _, d := range cw.globalMeta.Dictionaries {
		binary.Write(buf, binary.LittleEndian, d.Tag)
		binary.Write(buf, binary.LittleEndian, d.TID)
		binary.Write(buf, binary.LittleEndian, uint32(len(d.Entries)))
		for _, e := range d.Entries {
			WriteString(buf, e)
		}
	}
	return nil
}

func (cw *ContainerWriter) writeChunks(buf io.Writer) error {
	binary.Write(buf, binary.LittleEndian, uint32(len(cw.chunkData)))
	for _, cb := range cw.chunkData {
		binary.Write(buf, binary.LittleEndian, cb.entryIndex)
		binary.Write(buf, binary.LittleEndian, uint32(len(cb.data)))
		buf.Write(cb.data)
	}
	return nil
}

func (cw *ContainerWriter) writeFailedStreams(buf io.Writer) error {
	names := make([]string, 0, len(cw.failedData))
	for n := range cw.failedData {
		names = append(names, n)
	}
	binary.Write(buf, binary.LittleEndian, uint32(len(names)))
	for _, name := range names {
		fs := cw.failedData[name]
		WriteString(buf, name)
		binary.Write(buf, binary.LittleEndian, uint32(len(fs.LoadFailed)))
		for _, l := range fs.LoadFailed {
			binary.Write(buf, binary.LittleEndian, uint32(len(l)))
			buf.Write(l)
		}
		binary.Write(buf, binary.LittleEndian, uint32(len(fs.MatchFailed)))
		for _, l := range fs.MatchFailed {
			binary.Write(buf, binary.LittleEndian, uint32(len(l)))
			buf.Write(l)
		}
	}
	return nil
}

type ContainerReader struct {
	r           io.Reader
	Header      ContainerHeader
	Entries     []EntryDescriptor
	GlobalMeta  GlobalMeta
	Err         error
	Chunks      []ChunkEntry
	FailedLogs  map[string]FailedStream
}

func Open(r io.Reader) (*ContainerReader, error) {
	cr := &ContainerReader{FailedLogs: make(map[string]FailedStream)}

	magic := make([]byte, 4)
	if _, err := io.ReadFull(r, magic); err != nil {
		return nil, fmt.Errorf("read magic: %w", err)
	}

	var payload io.Reader
	if string(magic) == OuterMagic {
		var backendID uint8
		if err := binary.Read(r, binary.LittleEndian, &backendID); err != nil {
			return nil, fmt.Errorf("read backend id: %w", err)
		}
		var payloadLen uint64
		if err := binary.Read(r, binary.LittleEndian, &payloadLen); err != nil {
			return nil, fmt.Errorf("read payload len: %w", err)
		}
		compressed := make([]byte, payloadLen)
		if _, err := io.ReadFull(r, compressed); err != nil {
			return nil, fmt.Errorf("read compressed payload: %w", err)
		}
		comp, err := backendByID(backendID)
		if err != nil {
			return nil, err
		}
		compReader, err := comp.NewReader(bytes.NewReader(compressed))
		if err != nil {
			return nil, fmt.Errorf("decompress: %w", err)
		}
		defer compReader.Close()
		decompressed, err := io.ReadAll(compReader)
		if err != nil {
			return nil, fmt.Errorf("decompress read: %w", err)
		}
		payload = bytes.NewReader(decompressed)
		cr.Header.BackendID = backendID
	} else if string(magic) == Magic {
		var buf bytes.Buffer
		buf.Write(magic)
		io.Copy(&buf, r)
		payload = &buf
		cr.Header.BackendID = BackendNone
	} else {
		return nil, fmt.Errorf("unknown magic: %x", magic)
	}

	if err := cr.readContainer(payload); err != nil {
		return nil, err
	}
	return cr, nil
}

func (cr *ContainerReader) readContainer(r io.Reader) error {
	if err := cr.readHeader(r); err != nil {
		return err
	}
	if err := cr.readEntryTable(r); err != nil {
		return err
	}
	if err := cr.readGlobalMeta(r); err != nil {
		return err
	}
	if err := cr.readChunks(r); err != nil {
		return err
	}
	if err := cr.readFailedStreams(r); err != nil {
		return err
	}
	return nil
}

func (cr *ContainerReader) readHeader(r io.Reader) error {
	h := &cr.Header
	magic := make([]byte, 4)
	if _, err := io.ReadFull(r, magic); err != nil {
		return err
	}
	if string(magic) != Magic {
		return fmt.Errorf("bad container magic: %x", magic)
	}
	if err := binary.Read(r, binary.LittleEndian, &h.Version); err != nil {
		return err
	}
	if err := binary.Read(r, binary.LittleEndian, &h.Flags); err != nil {
		return err
	}
	if h.Version != Version {
		return fmt.Errorf("unsupported version: %d", h.Version)
	}
	if err := binary.Read(r, binary.LittleEndian, &h.BackendID); err != nil {
		return err
	}
	if err := binary.Read(r, binary.LittleEndian, &h.ChunkLines); err != nil {
		return err
	}
	if err := binary.Read(r, binary.LittleEndian, &h.WindowH); err != nil {
		return err
	}
	if h.Flags&FlagHasChecksum != 0 {
		if err := binary.Read(r, binary.LittleEndian, &h.Checksum); err != nil {
			return err
		}
	}
	return nil
}

func (cr *ContainerReader) readEntryTable(r io.Reader) error {
	var count uint32
	if err := binary.Read(r, binary.LittleEndian, &count); err != nil {
		return err
	}
	cr.Entries = make([]EntryDescriptor, count)
	for i := range cr.Entries {
		e := &cr.Entries[i]
		var err error
		e.Name, err = ReadString(r)
		if err != nil {
			return err
		}
		if err := binary.Read(r, binary.LittleEndian, &e.OrigSize); err != nil {
			return err
		}
		if err := binary.Read(r, binary.LittleEndian, &e.LineCount); err != nil {
			return err
		}
		if err := binary.Read(r, binary.LittleEndian, &e.ChunkCount); err != nil {
			return err
		}
		e.ChunkOffsets = make([]uint64, e.ChunkCount)
		for j := range e.ChunkOffsets {
			if err := binary.Read(r, binary.LittleEndian, &e.ChunkOffsets[j]); err != nil {
				return err
			}
		}
		if err := binary.Read(r, binary.LittleEndian, &e.FailedOff); err != nil {
			return err
		}
	}
	return nil
}

func (cr *ContainerReader) readGlobalMeta(r io.Reader) error {
	gm := &cr.GlobalMeta
	hf := &gm.HeadFormat
	if err := binary.Read(r, binary.LittleEndian, &hf.HeadLength); err != nil {
		return err
	}
	var isMulti uint8
	if err := binary.Read(r, binary.LittleEndian, &isMulti); err != nil {
		return err
	}
	hf.IsMulti = isMulti != 0
	var err error
	hf.HeadRegex, err = ReadString(r)
	if err != nil {
		return err
	}
	var fieldCount uint32
	if err := binary.Read(r, binary.LittleEndian, &fieldCount); err != nil {
		return err
	}
	hf.Fields = make([]HeaderFieldFormat, fieldCount)
	for i := range hf.Fields {
		f := &hf.Fields[i]
		if err := binary.Read(r, binary.LittleEndian, &f.StringSubs); err != nil {
			return err
		}
		if err := binary.Read(r, binary.LittleEndian, &f.NumericSubs); err != nil {
			return err
		}
		f.Format, err = ReadString(r)
		if err != nil {
			return err
		}
		var strLenCount uint8
		if err := binary.Read(r, binary.LittleEndian, &strLenCount); err != nil {
			return err
		}
		f.StrLens = make([]int8, strLenCount)
		for j := range f.StrLens {
			if err := binary.Read(r, binary.LittleEndian, &f.StrLens[j]); err != nil {
				return err
			}
		}
		var numLenCount uint8
		if err := binary.Read(r, binary.LittleEndian, &numLenCount); err != nil {
			return err
		}
		f.NumLens = make([]int8, numLenCount)
		for j := range f.NumLens {
			if err := binary.Read(r, binary.LittleEndian, &f.NumLens[j]); err != nil {
				return err
			}
		}
		f.Delim, err = ReadString(r)
		if err != nil {
			return err
		}
	}

	var tmplCount uint32
	if err := binary.Read(r, binary.LittleEndian, &tmplCount); err != nil {
		return err
	}
	gm.Templates = make([]Template, tmplCount)
	for i := range gm.Templates {
		var tokCount uint16
		if err := binary.Read(r, binary.LittleEndian, &tokCount); err != nil {
			return err
		}
		gm.Templates[i].Tokens = make([]TemplateToken, tokCount)
		for j := range gm.Templates[i].Tokens {
			tok := &gm.Templates[i].Tokens[j]
			if err := binary.Read(r, binary.LittleEndian, &tok.Kind); err != nil {
				return err
			}
			tok.Data, err = ReadString(r)
			if err != nil {
				return err
			}
		}
	}

	var dictCount uint32
	if err := binary.Read(r, binary.LittleEndian, &dictCount); err != nil {
		return err
	}
	gm.Dictionaries = make([]Dictionary, dictCount)
	for i := range gm.Dictionaries {
		d := &gm.Dictionaries[i]
		if err := binary.Read(r, binary.LittleEndian, &d.Tag); err != nil {
			return err
		}
		if d.Tag == 1 {
			if err := binary.Read(r, binary.LittleEndian, &d.TID); err != nil {
				return err
			}
		} else {
			if err := binary.Read(r, binary.LittleEndian, &d.TID); err != nil {
				return err
			}
		}
		var entryCount uint32
		if err := binary.Read(r, binary.LittleEndian, &entryCount); err != nil {
			return err
		}
		d.Entries = make([]string, entryCount)
		for j := range d.Entries {
			d.Entries[j], err = ReadString(r)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (cr *ContainerReader) readChunks(r io.Reader) error {
	var count uint32
	if err := binary.Read(r, binary.LittleEndian, &count); err != nil {
		return err
	}
	cr.Chunks = make([]ChunkEntry, count)
	for i := range cr.Chunks {
		ce := &cr.Chunks[i]
		if err := binary.Read(r, binary.LittleEndian, &ce.EntryIndex); err != nil {
			return err
		}
		var dataLen uint32
		if err := binary.Read(r, binary.LittleEndian, &dataLen); err != nil {
			return err
		}
		ce.Data = make([]byte, dataLen)
		if _, err := io.ReadFull(r, ce.Data); err != nil {
			return err
		}
	}
	return nil
}

func (cr *ContainerReader) readFailedStreams(r io.Reader) error {
	var nameCount uint32
	if err := binary.Read(r, binary.LittleEndian, &nameCount); err != nil {
		return err
	}
	for i := uint32(0); i < nameCount; i++ {
		name, err := ReadString(r)
		if err != nil {
			return err
		}
		var fs FailedStream
		var loadCount uint32
		if err := binary.Read(r, binary.LittleEndian, &loadCount); err != nil {
			return err
		}
		fs.LoadFailed = make([][]byte, loadCount)
		for j := range fs.LoadFailed {
			var l uint32
			if err := binary.Read(r, binary.LittleEndian, &l); err != nil {
				return err
			}
			fs.LoadFailed[j] = make([]byte, l)
			if _, err := io.ReadFull(r, fs.LoadFailed[j]); err != nil {
				return err
			}
		}
		var matchCount uint32
		if err := binary.Read(r, binary.LittleEndian, &matchCount); err != nil {
			return err
		}
		fs.MatchFailed = make([][]byte, matchCount)
		for j := range fs.MatchFailed {
			var l uint32
			if err := binary.Read(r, binary.LittleEndian, &l); err != nil {
				return err
			}
			fs.MatchFailed[j] = make([]byte, l)
			if _, err := io.ReadFull(r, fs.MatchFailed[j]); err != nil {
				return err
			}
		}
		cr.FailedLogs[name] = fs
	}
	return nil
}

func backendByID(id uint8) (backend.Compressor, error) {
	switch id {
	case BackendNone:
		return backend.Get("none")
	case BackendGzip:
		return backend.Get("gzip")
	case BackendZstd:
		return backend.Get("zstd")
	case BackendLzma:
		return nil, fmt.Errorf("lzma backend not implemented")
	default:
		return nil, fmt.Errorf("unknown backend id: %d", id)
	}
}


