package compress

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/sathyanarrayanan/logzip/internal/analyzer"
	"github.com/sathyanarrayanan/logzip/internal/cli"
	"github.com/sathyanarrayanan/logzip/internal/format"
	"github.com/sathyanarrayanan/logzip/internal/parser"
)

var ErrNoLogFiles = errors.New("no .log files found")

func Run(app *cli.App) error {
	if app.Options.Stdout {
		return compressToWriter(app.Args[0], os.Stdout, app.Options)
	}

	for _, path := range app.Args {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("cannot access %s: %w", path, err)
		}
		if info.IsDir() {
			if err := compressDir(path, app.Options); err != nil {
				return fmt.Errorf("compress dir %s: %w", path, err)
			}
		} else {
			if err := compressFile(path, app.Options); err != nil {
				return fmt.Errorf("compress %s: %w", path, err)
			}
		}
	}
	return nil
}

func compressFile(path string, opts *cli.Options) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	outPath := path + ".logz"
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	if opts.Verbose {
		fmt.Fprintf(os.Stderr, "compressing %s -> %s\n", path, outPath)
	}

	if err := compressStream(f, out, opts); err != nil {
		if !opts.Keep {
			os.Remove(outPath)
		}
		return err
	}

	if !opts.Keep {
		os.Remove(path)
	}
	return nil
}

func compressDir(path string, opts *cli.Options) error {
	var paths []string
	err := filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && !opts.Recursive && p != path {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".log") {
			paths = append(paths, p)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return ErrNoLogFiles
	}

	parent := filepath.Base(path)
	outPath := parent + ".logz"
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	if opts.Verbose {
		fmt.Fprintf(os.Stderr, "compressing %d .log files from %s/ -> %s\n", len(paths), path, outPath)
	}

	if err := compressMulti(paths, out, opts); err != nil {
		if !opts.Keep {
			os.Remove(outPath)
		}
		return err
	}

	return nil
}

func compressStream(r io.Reader, w io.Writer, opts *cli.Options) error {
	return compressImpl(r, w, opts, "")
}

func compressMulti(paths []string, w io.Writer, opts *cli.Options) error {
	return compressMultiImpl(paths, w, opts)
}

func compressToWriter(path string, w io.Writer, opts *cli.Options) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return compressStream(f, w, opts)
}

func compressImpl(r io.Reader, w io.Writer, opts *cli.Options, name string) error {
	headLen := 5
	drainDepth := 8
	st := 0.6

	hf, meta, err := ParseLogs(r, headLen, drainDepth, st)
	if err != nil {
		return fmt.Errorf("parse logs: %w", err)
	}

	if meta == nil || len(meta.EIDs) == 0 {
		return nil
	}

	var relations *analyzer.Relations

	globalMeta := buildGlobalMeta(hf, meta)
	chunkData := encodeChunkWithAnalyzer(meta, 0, len(meta.EIDs), headLen, relations)
	if name == "" {
		name = "input.log"
	}

	desc := format.EntryDescriptor{
		Name:        name,
		OrigSize:    uint64(len(chunkData)),
		LineCount:   uint64(len(meta.EIDs)),
		ChunkCount:  1,
		ChunkOffsets: []uint64{0},
	}

	cw := format.NewContainerWriter(w, &format.ContainerOptions{
		Backend:    opts.Backend,
		Level:      opts.Level,
		ChunkLines: opts.ChunkLines,
		Window:     opts.Window,
		Verify:     opts.Verify,
	})
	cw.SetGlobalMeta(globalMeta)
	cw.AddEntry(desc)
	cw.AddChunk(0, chunkData)

	return cw.Close()
}

func compressMultiImpl(paths []string, w io.Writer, opts *cli.Options) error {
	cw := format.NewContainerWriter(w, &format.ContainerOptions{
		Backend:    opts.Backend,
		Level:      opts.Level,
		ChunkLines: opts.ChunkLines,
		Window:     opts.Window,
		Verify:     opts.Verify,
	})

	var allGlobalMeta *format.GlobalMeta
	var descs []format.EntryDescriptor
	var allChunks [][]byte

	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open %s: %w", path, err)
		}
		hf, meta, err := ParseLogs(f, 5, 8, 0.6)
		f.Close()
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}

		if allGlobalMeta == nil {
			allGlobalMeta = buildGlobalMeta(hf, meta)
			cw.SetGlobalMeta(allGlobalMeta)
		}

		if meta == nil || len(meta.EIDs) == 0 {
			continue
		}

		chunkData := encodeChunk(meta, 0, len(meta.EIDs), 5)
		allChunks = append(allChunks, chunkData)

		relPath := filepath.Base(path)
		descs = append(descs, format.EntryDescriptor{
			Name:        relPath,
			OrigSize:    uint64(len(chunkData)),
			LineCount:   uint64(len(meta.EIDs)),
			ChunkCount:  1,
			ChunkOffsets: []uint64{uint64(len(allChunks) - 1)},
		})
	}

	for i, desc := range descs {
		cw.AddEntry(desc)
		cw.AddChunk(int32(i), allChunks[i])
	}

	return cw.Close()
}

func buildGlobalMeta(hf *parser.HeadFormat, meta *LogMeta) *format.GlobalMeta {
	gm := &format.GlobalMeta{}

	if hf != nil {
		gm.HeadFormat.HeadLength = hf.HeadLength
		gm.HeadFormat.IsMulti = hf.IsMulti
		gm.HeadFormat.HeadRegex = hf.HeadRegex
		gm.HeadFormat.Fields = make([]format.HeaderFieldFormat, len(hf.Fields))
		for i, f := range hf.Fields {
			gm.HeadFormat.Fields[i] = format.HeaderFieldFormat{
				StringSubs:  f.StringSubs,
				NumericSubs: f.NumericSubs,
				Format:      f.Format,
				StrLens:     f.StrLens,
				NumLens:     f.NumLens,
				Delim:       f.Delim,
			}
		}
	}

	gm.Templates = make([]format.Template, len(meta.Templates))
	for i, tmpl := range meta.Templates {
		toks := make([]format.TemplateToken, len(tmpl.Tokens))
		for j, tok := range tmpl.Tokens {
			kind := uint8(0)
			if tok.IsVar {
				kind = 1
			}
			toks[j] = format.TemplateToken{
				Kind: kind,
				Data: tok.Value,
			}
		}
		gm.Templates[i] = format.Template{Tokens: toks}
	}

	return gm
}
