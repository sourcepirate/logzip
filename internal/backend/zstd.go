package backend

import (
	"fmt"
	"io"

	"github.com/klauspost/compress/zstd"
)

type ZstdBackend struct{}

func (b *ZstdBackend) NewWriter(w io.Writer, level int) (io.WriteCloser, error) {
	lvl := zstd.SpeedDefault
	switch {
	case level <= 1:
		lvl = zstd.SpeedFastest
	case level >= 9:
		lvl = zstd.SpeedBestCompression
	case level <= 3:
		lvl = zstd.SpeedDefault
	case level <= 6:
		lvl = zstd.SpeedDefault
	default:
		lvl = zstd.SpeedDefault
	}
	zw, err := zstd.NewWriter(w, zstd.WithEncoderLevel(lvl))
	if err != nil {
		return nil, fmt.Errorf("zstd writer: %w", err)
	}
	return zw, nil
}

func (b *ZstdBackend) NewReader(r io.Reader) (io.ReadCloser, error) {
	zr, err := zstd.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("zstd reader: %w", err)
	}
	return zr.IOReadCloser(), nil
}

func (b *ZstdBackend) Name() string { return "zstd" }
func (b *ZstdBackend) ID() uint8    { return 2 }

func init() {
	Register(&ZstdBackend{})
}
