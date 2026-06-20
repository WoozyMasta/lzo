package lzo

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func testInputSet(t *testing.T) []struct {
	name string
	data []byte
} {
	t.Helper()

	inputs := []struct {
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

	entries, err := os.ReadDir(filepath.Join("testdata", "corpus"))
	if err != nil {
		t.Fatalf("ReadDir(testdata/corpus): %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".txt" {
			continue
		}

		data, err := os.ReadFile(filepath.Join("testdata", "corpus", entry.Name()))
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", entry.Name(), err)
		}
		inputs = append(inputs, struct {
			name string
			data []byte
		}{name: "corpus/" + entry.Name(), data: data})
	}

	return inputs
}

func TestCompressDecompress_RoundTripAcrossLevels(t *testing.T) {
	levels := []int{-7, 0, 1, 2, 5, 9, 15}

	for _, in := range testInputSet(t) {
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

func TestFastMatchLenMatchesBytewise(t *testing.T) {
	bytewise := func(in []byte, left, right int) int {
		start := right
		for right < len(in) && in[left] == in[right] {
			left++
			right++
		}
		return right - start
	}

	random := make([]byte, 1024)
	for i := range random {
		random[i] = byte((i*131 + i*i*17) >> 3)
	}
	for left := 0; left < 128; left++ {
		for right := left + 1; right < len(random); right += 7 {
			got := fastMatchLen(random, left, right)
			want := bytewise(random, left, right)
			if got != want {
				t.Fatalf("random left=%d right=%d: got %d, want %d", left, right, got, want)
			}
		}
	}

	for _, length := range []int{0, 1, 7, 8, 9, 15, 16, 31, 32, 63, 64, 127, 255} {
		data := make([]byte, 1024)
		for i := range data {
			data[i] = byte(i*29 + 7)
		}
		copy(data[512:512+length], data[16:16+length])
		if 512+length < len(data) {
			data[512+length] ^= 1
		}

		got := fastMatchLen(data, 16, 512)
		want := bytewise(data, 16, 512)
		if got != want {
			t.Fatalf("long match length=%d: got %d, want %d", length, got, want)
		}
	}

	overlap := bytes.Repeat([]byte{0x5a}, 1024)
	if got, want := fastMatchLen(overlap, 0, 1), len(overlap)-1; got != want {
		t.Fatalf("overlap: got %d, want %d", got, want)
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
	for _, in := range testInputSet(t) {
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

func TestEncoderMatchesPackageCompression(t *testing.T) {
	var encoder Encoder

	for _, in := range testInputSet(t) {
		for _, level := range []int{0, 1, 2, 5, 9, 15} {
			opts := &CompressOptions{Level: level}
			want, err := Compress(in.data, opts)
			if err != nil {
				t.Fatalf("Compress failed for %s level %d: %v", in.name, level, err)
			}

			dst := make([]byte, MaxCompressedSize(len(in.data)))
			got, err := encoder.CompressInto(in.data, dst, opts)
			if err != nil {
				t.Fatalf("Encoder.CompressInto failed for %s level %d: %v", in.name, level, err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("compressed output mismatch for %s level %d", in.name, level)
			}

			prefix := []byte("prefix:")
			appended, err := encoder.AppendCompress(append([]byte(nil), prefix...), in.data, opts)
			if err != nil {
				t.Fatalf("Encoder.AppendCompress failed for %s level %d: %v", in.name, level, err)
			}
			if !bytes.Equal(appended[:len(prefix)], prefix) || !bytes.Equal(appended[len(prefix):], want) {
				t.Fatalf("appended output mismatch for %s level %d", in.name, level)
			}
		}
	}
}

func TestEncoderCompressInto_BufferTooSmall(t *testing.T) {
	encoder := NewEncoder()
	data := bytes.Repeat([]byte("encoder-buffer"), 128)

	_, err := encoder.CompressInto(data, make([]byte, MaxCompressedSize(len(data))-1), &CompressOptions{Level: 9})
	if !errors.Is(err, ErrCompressBufferTooSmall) {
		t.Fatalf("expected ErrCompressBufferTooSmall, got %v", err)
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
	if got := MaxCompressedSize(math.MaxInt); got != -1 {
		t.Fatalf("MaxCompressedSize(math.MaxInt) = %d, want -1", got)
	}
}

func TestCompressBufferPoolRetentionLimit(t *testing.T) {
	for _, tc := range []struct {
		capacity int
		want     bool
	}{
		{capacity: hcMaxRetainedCompressBuffer - 1, want: true},
		{capacity: hcMaxRetainedCompressBuffer, want: true},
		{capacity: hcMaxRetainedCompressBuffer + 1, want: false},
	} {
		buf := &hcCompressBuffer{data: make([]byte, tc.capacity)}
		if got := canRetainCompressBuffer(buf); got != tc.want {
			t.Fatalf("capacity %d: canRetainCompressBuffer = %t, want %t", tc.capacity, got, tc.want)
		}
	}

	oversized := &hcCompressBuffer{data: make([]byte, hcMaxRetainedCompressBuffer+1)}
	releaseCompressBuffer(oversized)
	if oversized.data != nil {
		t.Fatal("oversized buffer still references its backing array")
	}
}

func TestCompress999GoldenOutput(t *testing.T) {
	inputs := []struct {
		name string
		data []byte
	}{
		{name: "mixed-256k", data: benchmarkMixedBytes(256 << 10)},
		{name: "random-256k", data: benchmarkRandomBytes(256 << 10)},
	}
	expected := map[string]string{
		"mixed-256k/level-2":  "b4b33f0c39190779da58b8eb5da040877e714f779f3d9dd829246c357e546056",
		"mixed-256k/level-5":  "502f8b26682a6c0ddf8f2a73b338c19a768d8f768e26f93f06f89d72d6597543",
		"mixed-256k/level-9":  "502f8b26682a6c0ddf8f2a73b338c19a768d8f768e26f93f06f89d72d6597543",
		"random-256k/level-2": "15abd887273e4bd85b5a7314abfd66364adc4e37a93ecae566e1c98894b65645",
		"random-256k/level-5": "15abd887273e4bd85b5a7314abfd66364adc4e37a93ecae566e1c98894b65645",
		"random-256k/level-9": "15abd887273e4bd85b5a7314abfd66364adc4e37a93ecae566e1c98894b65645",
	}

	for _, input := range inputs {
		for _, level := range []int{2, 5, 9} {
			name := fmt.Sprintf("%s/level-%d", input.name, level)
			compressed, err := Compress(input.data, &CompressOptions{Level: level})
			if err != nil {
				t.Fatalf("%s: Compress failed: %v", name, err)
			}

			got := fmt.Sprintf("%x", sha256.Sum256(compressed))
			if got != expected[name] {
				t.Errorf("%s: compressed SHA-256 = %s, want %s", name, got, expected[name])
			}
		}
	}
}

func TestCompress999RoundTripAfterDictionaryWrap(t *testing.T) {
	data := benchmarkRandomBytes(68551)
	compressed, err := Compress(data, &CompressOptions{Level: 9})
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	decoded, err := Decompress(compressed, DefaultDecompressOptions(len(data)))
	if err != nil {
		t.Fatalf("Decompress failed: %v", err)
	}
	if !bytes.Equal(decoded, data) {
		t.Fatalf("round-trip mismatch at byte %d", firstMismatchBytes(decoded, data))
	}
}

func firstMismatchBytes(left, right []byte) int {
	limit := min(len(left), len(right))
	for i := 0; i < limit; i++ {
		if left[i] != right[i] {
			return i
		}
	}
	return limit
}

func FuzzCompressDecompressRoundTrip(f *testing.F) {
	f.Add([]byte(""), uint8(0))
	f.Add([]byte("hello world"), uint8(1))
	f.Add(bytes.Repeat([]byte{0x00}, 1024), uint8(9))
	f.Add(bytes.Repeat([]byte("abc"), 500), uint8(7))

	// Seeds large enough to cycle the 999-path ring window at least once
	// (hcMaxDist = 49151). Ring-slot reuse bugs require input longer than the
	// window to manifest, so these sizes are intentional.
	ringOnce := make([]byte, hcMaxDist+4096)
	for i := range ringOnce {
		ringOnce[i] = byte(i*17 + 3)
	}
	f.Add(ringOnce, uint8(9))

	ringTwice := make([]byte, hcMaxDist*2+1024)
	for i := range ringTwice {
		ringTwice[i] = byte(i * 251)
	}
	f.Add(ringTwice, uint8(5))

	f.Add(bytes.Repeat([]byte{0x00}, hcMaxDist+1), uint8(9))
	f.Add(bytes.Repeat([]byte("xy"), hcMaxDist/2+1), uint8(7))

	f.Fuzz(func(t *testing.T, data []byte, level uint8) {
		// Cap at 3× the ring window so double ring-wrap is always reachable.
		if len(data) > hcMaxDist*3 {
			data = data[:hcMaxDist*3]
		}

		lvl := int(level % 10)

		cmp, err := Compress(data, &CompressOptions{Level: lvl})
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
		into, err := CompressInto(data, dst, &CompressOptions{Level: lvl})
		if err != nil {
			t.Fatalf("CompressInto failed: %v", err)
		}
		if !bytes.Equal(into, cmp) {
			t.Fatal("CompressInto output differs from Compress")
		}

		appended, err := AppendCompress(nil, data, &CompressOptions{Level: lvl})
		if err != nil {
			t.Fatalf("AppendCompress failed: %v", err)
		}
		if !bytes.Equal(appended, cmp) {
			t.Fatal("AppendCompress output differs from Compress")
		}
	})
}
