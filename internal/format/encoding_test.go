package format

import (
	"bytes"
	"testing"
)

func TestZigzag32(t *testing.T) {
	cases := []struct {
		in  int32
		out uint32
	}{
		{0, 0},
		{-1, 1},
		{1, 2},
		{-2, 3},
		{2, 4},
		{2147483647, 4294967294},
		{-2147483648, 4294967295},
	}
	for _, c := range cases {
		got := Zigzag32(c.in)
		if got != c.out {
			t.Errorf("Zigzag32(%d) = %d, want %d", c.in, got, c.out)
		}
		back := Unzigzag32(got)
		if back != c.in {
			t.Errorf("Unzigzag32(%d) = %d, want %d", got, back, c.in)
		}
	}
}

func TestZigzag64(t *testing.T) {
	cases := []struct {
		in  int64
		out uint64
	}{
		{0, 0},
		{-1, 1},
		{1, 2},
		{-2, 3},
		{9223372036854775807, 18446744073709551614},
		{-9223372036854775808, 18446744073709551615},
	}
	for _, c := range cases {
		got := Zigzag64(c.in)
		if got != c.out {
			t.Errorf("Zigzag64(%d) = %d, want %d", c.in, got, c.out)
		}
		back := Unzigzag64(got)
		if back != c.in {
			t.Errorf("Unzigzag64(%d) = %d, want %d", got, back, c.in)
		}
	}
}

func TestVarint32RoundTrip(t *testing.T) {
	vals := []int32{0, 1, -1, 127, -128, 1000, -1000, 1 << 20, -(1 << 20), 1 << 30, -(1 << 30)}
	for _, v := range vals {
		buf := make([]byte, 10)
		n := PutVarint32(buf, v)
		r := bytes.NewReader(buf[:n])
		got, err := ReadVarint32(r)
		if err != nil {
			t.Errorf("ReadVarint32(%d): %v", v, err)
		}
		if got != v {
			t.Errorf("round trip %d: got %d", v, got)
		}
	}
}

func TestVarint64RoundTrip(t *testing.T) {
	vals := []int64{0, 1, -1, 1 << 40, -(1 << 40), 1 << 62, -(1 << 62)}
	for _, v := range vals {
		buf := make([]byte, 10)
		n := PutVarint64(buf, v)
		r := bytes.NewReader(buf[:n])
		got, err := ReadVarint64(r)
		if err != nil {
			t.Errorf("ReadVarint64(%d): %v", v, err)
		}
		if got != v {
			t.Errorf("round trip %d: got %d", v, got)
		}
	}
}

func TestWriteReadString(t *testing.T) {
	var buf bytes.Buffer
	s := "hello, logzip!"
	n, err := WriteString(&buf, s)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2+len(s) {
		t.Errorf("wrote %d bytes, expected %d", n, 2+len(s))
	}
	got, err := ReadString(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got != s {
		t.Errorf("round trip %q: got %q", s, got)
	}
}

func TestVarintSize(t *testing.T) {
	cases := []int32{0, 1, 63, 64, 127, 128, 16383, 16384, 2097151, 2097152, 268435455, 268435456}
	for _, v := range cases {
		buf := make([]byte, 10)
		n := PutVarint32(buf, v)
		sz := Varint32Size(v)
		if n != sz {
			t.Errorf("Varint32Size(%d) = %d, actual encoded size = %d", v, sz, n)
		}
	}
}

func BenchmarkVarint32(b *testing.B) {
	buf := make([]byte, 10)
	for i := 0; i < b.N; i++ {
		PutVarint32(buf, int32(i))
	}
}

func BenchmarkReadVarint32(b *testing.B) {
	vals := make([]byte, b.N*10)
	n := 0
	for i := 0; i < b.N; i++ {
		n += PutVarint32(vals[n:], int32(i))
	}
	r := bytes.NewReader(vals)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ReadVarint32(r)
	}
}
