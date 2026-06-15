package lzo

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
)

func testInputSet() []struct {
	name string
	data []byte
} {
	return []struct {
		name string
		data []byte
	}{
		{name: "nil", data: nil},
		{name: "empty", data: []byte{}},
		{name: "single-byte", data: []byte{0xAB}},
		{name: "short-text", data: []byte("hello world, lzo test")},
		{name: "repeated-pattern", data: bytes.Repeat([]byte("abc123"), 2000)},
		{name: "long-run", data: bytes.Repeat([]byte{0xFF}, 12000)},
		{name: "byte-cycle", data: bytes.Repeat([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, 1200)},
	}
}

func TestCompressDecompress_RoundTripAcrossLevels(t *testing.T) {
	levels := []int{-7, 0, 1, 2, 5, 9, 15}

	for _, in := range testInputSet() {
		for _, level := range levels {
			name := fmt.Sprintf("%s/level-%d", in.name, level)
			t.Run(name, func(t *testing.T) {
				cmp, err := Compress(in.data, &CompressOptions{Level: level})
				if err != nil {
					t.Fatalf("Compress failed: %v", err)
				}
				if len(cmp) < 3 {
					t.Fatalf("compressed data too short: %d", len(cmp))
				}
				if !bytes.Equal(cmp[len(cmp)-3:], []byte{markerM4 | 1, 0, 0}) {
					t.Fatalf("missing stream terminator: % x", cmp[len(cmp)-3:])
				}

				out, err := Decompress(cmp, DefaultDecompressOptions(len(in.data)))
				if err != nil {
					t.Fatalf("Decompress failed: %v", err)
				}
				if !bytes.Equal(out, in.data) {
					t.Fatalf("round-trip mismatch: got=%d want=%d", len(out), len(in.data))
				}

				outReader, err := DecompressFromReader(bytes.NewReader(cmp), DefaultDecompressOptions(len(in.data)))
				if err != nil {
					t.Fatalf("DecompressFromReader failed: %v", err)
				}
				if !bytes.Equal(outReader, in.data) {
					t.Fatalf("reader round-trip mismatch: got=%d want=%d", len(outReader), len(in.data))
				}
			})
		}
	}
}

func TestCompress_DefaultAndExplicitLevels(t *testing.T) {
	data := bytes.Repeat([]byte("ABCDEF123456"), 1024)

	cmpDefault, err := Compress(data, nil)
	if err != nil {
		t.Fatalf("Compress default failed: %v", err)
	}

	cmpLevel1, err := Compress(data, &CompressOptions{Level: 1})
	if err != nil {
		t.Fatalf("Compress level=1 failed: %v", err)
	}

	cmpLevel0, err := Compress(data, &CompressOptions{Level: 0})
	if err != nil {
		t.Fatalf("Compress level=0 failed: %v", err)
	}

	if !bytes.Equal(cmpDefault, cmpLevel1) {
		t.Fatal("default compression should match level=1")
	}
	if !bytes.Equal(cmpLevel0, cmpLevel1) {
		t.Fatal("level=0 and level=1 should use identical fast compressor")
	}
}

func TestCompress_LevelClamping(t *testing.T) {
	data := bytes.Repeat([]byte("0123456789abcdef"), 4096)

	cmpNeg, err := Compress(data, &CompressOptions{Level: -100})
	if err != nil {
		t.Fatalf("Compress level=-100 failed: %v", err)
	}
	cmpZero, err := Compress(data, &CompressOptions{Level: 0})
	if err != nil {
		t.Fatalf("Compress level=0 failed: %v", err)
	}
	if !bytes.Equal(cmpNeg, cmpZero) {
		t.Fatal("negative level should be clamped to level 0")
	}

	cmpHigh, err := Compress(data, &CompressOptions{Level: 100})
	if err != nil {
		t.Fatalf("Compress level=100 failed: %v", err)
	}
	cmpNine, err := Compress(data, &CompressOptions{Level: 9})
	if err != nil {
		t.Fatalf("Compress level=9 failed: %v", err)
	}
	if !bytes.Equal(cmpHigh, cmpNine) {
		t.Fatal("level > 9 should be clamped to level 9")
	}
}

func TestCompress1X999Level_LevelClamping(t *testing.T) {
	data := bytes.Repeat([]byte("compress-999-level"), 512)

	cmpLow, err := Compress1X999Level(data, -10)
	if err != nil {
		t.Fatalf("Compress1X999Level(-10) failed: %v", err)
	}

	cmpOne, err := Compress1X999Level(data, 1)
	if err != nil {
		t.Fatalf("Compress1X999Level(1) failed: %v", err)
	}

	if !bytes.Equal(cmpLow, cmpOne) {
		t.Fatal("level < 1 should clamp to level 1")
	}

	cmpHigh, err := Compress1X999Level(data, 100)
	if err != nil {
		t.Fatalf("Compress1X999Level(100) failed: %v", err)
	}

	cmpNine, err := Compress1X999Level(data, 9)
	if err != nil {
		t.Fatalf("Compress1X999Level(9) failed: %v", err)
	}

	if !bytes.Equal(cmpHigh, cmpNine) {
		t.Fatal("level > 9 should clamp to level 9")
	}

	out, err := Decompress(cmpNine, DefaultDecompressOptions(len(data)))
	if err != nil {
		t.Fatalf("Decompress of Compress1X999Level output failed: %v", err)
	}

	if !bytes.Equal(out, data) {
		t.Fatal("round-trip mismatch for Compress1X999Level")
	}
}

func TestCompressIntoMatchesCompress(t *testing.T) {
	for _, in := range testInputSet() {
		for _, level := range []int{-7, 0, 1, 2, 5, 9, 15} {
			opts := &CompressOptions{Level: level}
			want, err := Compress(in.data, opts)
			if err != nil {
				t.Fatalf("Compress failed for %s level %d: %v", in.name, level, err)
			}

			dst := make([]byte, MaxCompressedSize(len(in.data)))
			got, err := CompressInto(in.data, dst, opts)
			if err != nil {
				t.Fatalf("CompressInto failed for %s level %d: %v", in.name, level, err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("compressed output mismatch for %s level %d", in.name, level)
			}
			if len(got) > 0 && &got[0] != &dst[0] {
				t.Fatalf("CompressInto did not reuse dst for %s level %d", in.name, level)
			}
		}
	}
}

func TestCompressInto_BufferTooSmall(t *testing.T) {
	data := bytes.Repeat([]byte("small-output"), 1024)
	dst := make([]byte, MaxCompressedSize(len(data))-1)

	for _, level := range []int{1, 9} {
		_, err := CompressInto(data, dst, &CompressOptions{Level: level})
		if !errors.Is(err, ErrCompressBufferTooSmall) {
			t.Fatalf("level %d: expected ErrCompressBufferTooSmall, got %v", level, err)
		}
	}
}

func TestAppendCompressPreservesPrefixAndReusesCapacity(t *testing.T) {
	data := bytes.Repeat([]byte("append-compress"), 1024)
	prefix := []byte("prefix:")

	for _, level := range []int{1, 9} {
		opts := &CompressOptions{Level: level}
		want, err := Compress(data, opts)
		if err != nil {
			t.Fatalf("Compress failed for level %d: %v", level, err)
		}

		dst := make([]byte, len(prefix), len(prefix)+MaxCompressedSize(len(data)))
		copy(dst, prefix)
		got, err := AppendCompress(dst, data, opts)
		if err != nil {
			t.Fatalf("AppendCompress failed for level %d: %v", level, err)
		}
		if !bytes.Equal(got[:len(prefix)], prefix) {
			t.Fatalf("prefix changed for level %d", level)
		}
		if !bytes.Equal(got[len(prefix):], want) {
			t.Fatalf("compressed output mismatch for level %d", level)
		}
		if &got[0] != &dst[0] {
			t.Fatalf("AppendCompress did not reuse dst capacity for level %d", level)
		}
	}
}

func TestAppendCompressGrowsDestination(t *testing.T) {
	data := bytes.Repeat([]byte("grow-destination"), 1024)
	prefix := []byte("prefix:")
	want, err := Compress(data, nil)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	got, err := AppendCompress(append([]byte(nil), prefix...), data, nil)
	if err != nil {
		t.Fatalf("AppendCompress failed: %v", err)
	}
	if !bytes.Equal(got[len(prefix):], want) {
		t.Fatal("compressed output mismatch")
	}

	out, err := Decompress(got[len(prefix):], DefaultDecompressOptions(len(data)))
	if err != nil {
		t.Fatalf("Decompress failed: %v", err)
	}
	if !bytes.Equal(out, data) {
		t.Fatal("round-trip mismatch")
	}
}

func TestMaxCompressedSize(t *testing.T) {
	for _, size := range []int{0, 1, 16, 1 << 20} {
		if got := MaxCompressedSize(size); got < size+3 {
			t.Fatalf("MaxCompressedSize(%d) = %d, want at least %d", size, got, size+3)
		}
	}
	if got := MaxCompressedSize(-1); got != -1 {
		t.Fatalf("MaxCompressedSize(-1) = %d, want -1", got)
	}
	if got := MaxCompressedSize(maxInt); got != -1 {
		t.Fatalf("MaxCompressedSize(maxInt) = %d, want -1", got)
	}
}

func FuzzCompressDecompressRoundTrip(f *testing.F) {
	f.Add([]byte(""), uint8(0))
	f.Add([]byte("hello world"), uint8(1))
	f.Add(bytes.Repeat([]byte{0x00}, 1024), uint8(9))
	f.Add(bytes.Repeat([]byte("abc"), 500), uint8(7))

	f.Fuzz(func(t *testing.T, data []byte, level uint8) {
		if len(data) > 1<<16 {
			data = data[:1<<16]
		}

		cmp, err := Compress(data, &CompressOptions{Level: int(level % 16)})
		if err != nil {
			t.Fatalf("Compress failed: %v", err)
		}

		out, err := Decompress(cmp, DefaultDecompressOptions(len(data)))
		if err != nil {
			t.Fatalf("Decompress failed: %v", err)
		}

		if !bytes.Equal(out, data) {
			t.Fatalf("round-trip mismatch: got=%d want=%d", len(out), len(data))
		}

		dst := make([]byte, MaxCompressedSize(len(data)))
		into, err := CompressInto(data, dst, &CompressOptions{Level: int(level % 16)})
		if err != nil {
			t.Fatalf("CompressInto failed: %v", err)
		}
		if !bytes.Equal(into, cmp) {
			t.Fatal("CompressInto output differs from Compress")
		}
	})
}
