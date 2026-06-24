package format

import (
	"encoding/binary"
	"errors"
	"io"
)

func Zigzag32(n int32) uint32 {
	return uint32((n << 1) ^ (n >> 31))
}

func Unzigzag32(n uint32) int32 {
	return int32((n >> 1) ^ -(n & 1))
}

func Zigzag64(n int64) uint64 {
	return uint64((n << 1) ^ (n >> 63))
}

func Unzigzag64(n uint64) int64 {
	return int64((n >> 1) ^ -(n & 1))
}

func PutVarint32(buf []byte, n int32) int {
	return binary.PutUvarint(buf, uint64(Zigzag32(n)))
}

func PutVarint64(buf []byte, n int64) int {
	return binary.PutUvarint(buf, Zigzag64(n))
}

func ReadVarint32(r io.ByteReader) (int32, error) {
	v, err := binary.ReadUvarint(r)
	if err != nil {
		return 0, err
	}
	return Unzigzag32(uint32(v)), nil
}

func ReadVarint64(r io.ByteReader) (int64, error) {
	v, err := binary.ReadUvarint(r)
	if err != nil {
		return 0, err
	}
	return Unzigzag64(v), nil
}

func Varint32Len(n int32) int {
	buf := make([]byte, binary.MaxVarintLen32)
	return binary.PutUvarint(buf, uint64(Zigzag32(n)))
}

func Varint64Len(n int64) int {
	buf := make([]byte, binary.MaxVarintLen64)
	return binary.PutUvarint(buf, Zigzag64(n))
}

func Varint32Size(n int32) int {
	buf := make([]byte, binary.MaxVarintLen32)
	return binary.PutUvarint(buf, uint64(Zigzag32(n)))
}

type IntReader struct {
	reader io.ByteReader
}

func NewIntReader(r io.ByteReader) *IntReader {
	return &IntReader{reader: r}
}

func (r *IntReader) Read() (int32, error) {
	return ReadVarint32(r.reader)
}

func PutUvarint(buf []byte, n uint64) int {
	return binary.PutUvarint(buf, n)
}

func ReadUvarint(r io.ByteReader) (uint64, error) {
	return binary.ReadUvarint(r)
}

var ErrOverflow = errors.New("varint overflow")
