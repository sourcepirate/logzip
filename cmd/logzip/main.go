package main

import (
	"fmt"
	"os"

	"github.com/sathyanarrayanan/logzip/internal/cli"
	"github.com/sathyanarrayanan/logzip/internal/compress"
	"github.com/sathyanarrayanan/logzip/internal/decompress"
)

const version = "0.1.0"

func main() {
	opts, args, err := cli.ParseFlags(os.Args[1:])
	if err != nil {
		if err.Error() == "flag: help requested" {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(2)
	}

	if opts.ShowVersion {
		fmt.Printf("logzip %s (LogShrink algorithm)\n", version)
		os.Exit(0)
	}

	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no input files\n")
		os.Exit(2)
	}

	app := cli.NewApp(opts, args)

	if opts.Decompress || opts.Test {
		if err := decompress.Run(app); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	} else if opts.List {
		if err := decompress.List(app); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	} else {
		if err := compress.Run(app); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}
}
