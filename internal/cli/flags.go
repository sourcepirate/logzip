package cli

import (
	"flag"
	"fmt"
	"os"
	"runtime"
)

type Options struct {
	Decompress   bool
	Stdout       bool
	Keep         bool
	Force        bool
	Recursive    bool
	Level        int
	Backend      string
	NoBackend    bool
	ChunkLines   int
	Window       int
	Verbose      bool
	Quiet        bool
	Verify       bool
	Jobs         int
	ShowVersion  bool
	List         bool
	Test         bool
}

func ParseFlags(args []string) (*Options, []string, error) {
	opts := &Options{}
	fs := flag.NewFlagSet("logzip", flag.ContinueOnError)
	fs.BoolVar(&opts.Decompress, "d", false, "decompress")
	fs.BoolVar(&opts.Decompress, "decompress", false, "")
	fs.BoolVar(&opts.Stdout, "c", false, "write to stdout")
	fs.BoolVar(&opts.Stdout, "stdout", false, "")
	fs.BoolVar(&opts.Keep, "k", false, "keep original file")
	fs.BoolVar(&opts.Keep, "keep", false, "")
	fs.BoolVar(&opts.Force, "f", false, "force overwrite output")
	fs.BoolVar(&opts.Force, "force", false, "")
	fs.BoolVar(&opts.Recursive, "r", false, "recursively search directories")
	fs.BoolVar(&opts.Recursive, "recursive", false, "")
	fs.IntVar(&opts.Level, "level", 6, "compression level (1-9)")
	fs.StringVar(&opts.Backend, "backend", "zstd", "backend compressor: zstd, gzip, lzma, none")
	fs.BoolVar(&opts.NoBackend, "no-backend", false, "disable backend pass")
	fs.IntVar(&opts.ChunkLines, "chunk-lines", 100000, "lines per chunk")
	fs.IntVar(&opts.Window, "window", 20, "sequence window length")
	fs.BoolVar(&opts.Verbose, "v", false, "verbose output")
	fs.BoolVar(&opts.Verbose, "verbose", false, "")
	fs.BoolVar(&opts.Quiet, "q", false, "quiet output")
	fs.BoolVar(&opts.Quiet, "quiet", false, "")
	fs.BoolVar(&opts.Verify, "verify", true, "verify round-trip after compression")
	fs.IntVar(&opts.Jobs, "j", runtime.GOMAXPROCS(0), "parallel workers")
	fs.BoolVar(&opts.ShowVersion, "version", false, "print version")
	fs.BoolVar(&opts.List, "l", false, "list archive contents")
	fs.BoolVar(&opts.Test, "t", false, "test archive integrity")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `logzip — log-specialized lossless compressor (LogShrink algorithm)

USAGE:
  logzip [compress|decompress] [OPTIONS] FILE...
  logzip -d [OPTIONS] FILE.logz

EXAMPLES:
  logzip app.log                 -> app.logz (replaces app.log)
  logzip -k app.log              -> app.logz (keeps app.log)
  logzip -r -k logs/             -> logs.logz
  logzip -d app.logz             -> app.log
  logzip -dc app.logz | grep ERR # decompress to stdout
  logzip -t app.logz             # verify integrity

OPTIONS:
`)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return nil, nil, err
	}

	if opts.NoBackend {
		opts.Backend = "none"
	}

	if opts.Level < 1 {
		opts.Level = 1
	} else if opts.Level > 9 {
		opts.Level = 9
	}
	if opts.ChunkLines < 100 {
		opts.ChunkLines = 100
	}
	if opts.Window < 2 {
		opts.Window = 2
	}

	remaining := fs.Args()
	if len(remaining) > 0 && (remaining[0] == "compress" || remaining[0] == "c") {
		opts.Decompress = false
		remaining = remaining[1:]
	} else if len(remaining) > 0 && (remaining[0] == "decompress" || remaining[0] == "d") {
		opts.Decompress = true
		remaining = remaining[1:]
	}

	return opts, remaining, nil
}

type App struct {
	Options *Options
	Args    []string
}

func NewApp(opts *Options, args []string) *App {
	return &App{Options: opts, Args: args}
}
