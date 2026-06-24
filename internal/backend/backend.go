package backend

import (
	"compress/gzip"
	"fmt"
	"io"
)

type Compressor interface {
	NewWriter(w io.Writer, level int) (io.WriteCloser, error)
	NewReader(r io.Reader) (io.ReadCloser, error)
	Name() string
	ID() uint8
}

var backends = map[string]Compressor{
	"gzip": &GzipBackend{},
	"none": &NoneBackend{},
}

type GzipBackend struct{}

func (b *GzipBackend) NewWriter(w io.Writer, level int) (io.WriteCloser, error) {
	lvl := gzip.DefaultCompression
	if level >= 1 && level <= 9 {
		lvl = level
	}
	zw, err := gzip.NewWriterLevel(w, lvl)
	if err != nil {
		return nil, err
	}
	return zw, nil
}

func (b *GzipBackend) NewReader(r io.Reader) (io.ReadCloser, error) {
	return gzip.NewReader(r)
}

func (b *GzipBackend) Name() string { return "gzip" }
func (b *GzipBackend) ID() uint8    { return 1 }

type NoneBackend struct{}

func (b *NoneBackend) NewWriter(w io.Writer, level int) (io.WriteCloser, error) {
	return nopCloser{w}, nil
}

func (b *NoneBackend) NewReader(r io.Reader) (io.ReadCloser, error) {
	return io.NopCloser(r), nil
}

func (b *NoneBackend) Name() string { return "none" }
func (b *NoneBackend) ID() uint8    { return 0 }

type nopCloser struct {
	io.Writer
}

func (n nopCloser) Close() error { return nil }

func Get(name string) (Compressor, error) {
	b, ok := backends[name]
	if !ok {
		return nil, fmt.Errorf("unknown backend: %s", name)
	}
	return b, nil
}

func Register(b Compressor) {
	backends[b.Name()] = b
}
