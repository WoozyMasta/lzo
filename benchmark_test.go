package lzo

import (
	"bytes"
	"fmt"
	"testing"
)

type benchmarkInput struct {
	name string
	data []byte
}

var benchmarkLevels = [...]int{1, 5, 9}

func benchmarkRandomBytes(size int) []byte {
	data := make([]byte, size)
	state := uint64(0x9e3779b97f4a7c15)
	for i := range data {
		state ^= state << 13
		state ^= state >> 7
		state ^= state << 17
		data[i] = byte(state)
	}
	return data
}

func benchmarkMixedBytes(size int) []byte {
	data := benchmarkRandomBytes(size)
	pattern := []byte("level=info component=lzo message=benchmark payload=0123456789abcdef\n")
	for start := 0; start < len(data); start += 4096 {
		end := min(start+3072, len(data))
		for pos := start; pos < end; pos += len(pattern) {
			copy(data[pos:end], pattern)
		}
	}
	return data
}

func benchmarkTokenHeavyBytes(size int) []byte {
	data := make([]byte, size)
	prefixes := benchmarkRandomBytes(64 * 4)
	random := benchmarkRandomBytes(size)
	for pos := 0; pos < len(data); pos += 8 {
		prefix := ((pos / 8) * 29 % 64) * 4
		n := min(4, len(data)-pos)
		copy(data[pos:pos+n], prefixes[prefix:prefix+n])
		if pos+n < len(data) {
			copy(data[pos+n:min(pos+8, len(data))], random[pos+n:min(pos+8, len(data))])
		}
	}
	return data
}

func benchmarkCompressionInputs() []benchmarkInput {
	return []benchmarkInput{
		{name: "small-text-4k", data: bytes.Repeat([]byte("lzo benchmark text payload "), 160)},
		{name: "pattern-128k", data: bytes.Repeat([]byte("ABCDEF0123456789"), 8192)},
		{name: "byte-cycle-256k", data: bytes.Repeat([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, 26214)},
		{name: "mixed-256k", data: benchmarkMixedBytes(256 << 10)},
		{name: "random-256k", data: benchmarkRandomBytes(256 << 10)},
	}
}

func benchmarkDecompressionInputs() []benchmarkInput {
	inputs := benchmarkCompressionInputs()
	return append(inputs, benchmarkInput{
		name: "token-heavy-256k",
		data: benchmarkTokenHeavyBytes(256 << 10),
	})
}

func reportCompressionMetrics(b *testing.B, compressed, input []byte) {
	b.ReportMetric(float64(len(compressed)), "compressed-B")
	if len(input) > 0 {
		b.ReportMetric(float64(len(compressed))*100/float64(len(input)), "ratio-%")
	}
}

func BenchmarkCompress(b *testing.B) {
	for _, input := range benchmarkCompressionInputs() {
		for _, level := range benchmarkLevels {
			name := fmt.Sprintf("%s/level-%d", input.name, level)
			b.Run(name, func(b *testing.B) {
				opts := &CompressOptions{Level: level}
				compressed, err := Compress(input.data, opts)
				if err != nil {
					b.Fatalf("setup Compress failed: %v", err)
				}

				b.ReportAllocs()
				b.SetBytes(int64(len(input.data)))
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					_, err := Compress(input.data, opts)
					if err != nil {
						b.Fatalf("Compress failed: %v", err)
					}
				}

				reportCompressionMetrics(b, compressed, input.data)
			})
		}
	}
}

func BenchmarkCompressInto(b *testing.B) {
	benchmarkCompressCallerBuffer(b, false)
}

func BenchmarkAppendCompress(b *testing.B) {
	benchmarkCompressCallerBuffer(b, true)
}

func benchmarkCompressCallerBuffer(b *testing.B, appendMode bool) {
	for _, input := range benchmarkCompressionInputs() {
		for _, level := range benchmarkLevels {
			name := fmt.Sprintf("%s/level-%d", input.name, level)
			b.Run(name, func(b *testing.B) {
				opts := &CompressOptions{Level: level}
				dst := make([]byte, MaxCompressedSize(len(input.data)))
				var compressed []byte
				var err error

				b.ReportAllocs()
				b.SetBytes(int64(len(input.data)))
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					if appendMode {
						compressed, err = AppendCompress(dst[:0], input.data, opts)
					} else {
						compressed, err = CompressInto(input.data, dst, opts)
					}
					if err != nil {
						b.Fatalf("compression failed: %v", err)
					}
				}

				reportCompressionMetrics(b, compressed, input.data)
			})
		}
	}
}

func BenchmarkCompress999Core(b *testing.B) {
	input := benchmarkMixedBytes(256 << 10)
	dict := &hcCompressorDict{}
	out := make([]byte, MaxCompressedSize(len(input)))

	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := compress999NoAlloc(input, out, dict, 9); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecompress(b *testing.B) {
	for _, input := range benchmarkDecompressionInputs() {
		for _, level := range benchmarkLevels {
			compressedData, err := Compress(input.data, &CompressOptions{Level: level})
			if err != nil {
				b.Fatalf("setup Compress failed for %s level %d: %v", input.name, level, err)
			}

			opts := DefaultDecompressOptions(len(input.data))
			if _, err := Decompress(compressedData, opts); err != nil {
				b.Fatalf("setup Decompress failed for %s level %d: %v", input.name, level, err)
			}

			name := fmt.Sprintf("%s/from-level-%d", input.name, level)
			b.Run(name, func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(input.data)))
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					_, err := Decompress(compressedData, opts)
					if err != nil {
						b.Fatalf("Decompress failed: %v", err)
					}
				}
			})
		}
	}
}

func BenchmarkDecompressInto(b *testing.B) {
	for _, input := range benchmarkDecompressionInputs() {
		for _, level := range benchmarkLevels {
			compressedData, err := Compress(input.data, &CompressOptions{Level: level})
			if err != nil {
				b.Fatalf("setup Compress failed for %s level %d: %v", input.name, level, err)
			}

			dst := make([]byte, len(input.data))
			if _, err := DecompressInto(compressedData, dst); err != nil {
				b.Fatalf("setup DecompressInto failed for %s level %d: %v", input.name, level, err)
			}

			name := fmt.Sprintf("%s/from-level-%d", input.name, level)
			b.Run(name, func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(input.data)))
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					_, err := DecompressInto(compressedData, dst)
					if err != nil {
						b.Fatalf("DecompressInto failed: %v", err)
					}
				}
			})
		}
	}
}

func BenchmarkDecompressFromReader(b *testing.B) {
	benchmarkDecompressFromReader(b, false)
}

func BenchmarkDecompressFromReaderLimited(b *testing.B) {
	benchmarkDecompressFromReader(b, true)
}

func benchmarkDecompressFromReader(b *testing.B, limited bool) {
	for _, input := range benchmarkDecompressionInputs() {
		compressed, err := Compress(input.data, &CompressOptions{Level: 1})
		if err != nil {
			b.Fatalf("setup Compress failed for %s: %v", input.name, err)
		}

		opts := DefaultDecompressOptions(len(input.data))
		if limited {
			opts.MaxInputSize = len(compressed)
		}
		reader := bytes.NewReader(compressed)
		b.Run(input.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(input.data)))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				reader.Reset(compressed)
				if _, err := DecompressFromReader(reader, opts); err != nil {
					b.Fatalf("DecompressFromReader failed: %v", err)
				}
			}
		})
	}
}

func BenchmarkRoundTrip(b *testing.B) {
	inputData := bytes.Repeat([]byte("RoundTripData"), 16384)
	opts := &CompressOptions{Level: 9}
	b.ReportAllocs()
	b.SetBytes(int64(len(inputData)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		compressedData, err := Compress(inputData, opts)
		if err != nil {
			b.Fatalf("Compress failed: %v", err)
		}
		_, err = Decompress(compressedData, DefaultDecompressOptions(len(inputData)))
		if err != nil {
			b.Fatalf("Decompress failed: %v", err)
		}
	}
}

func BenchmarkCopyBackRef(b *testing.B) {
	const outputPos = 64 << 10
	dst := make([]byte, outputPos+(64<<10))

	for _, tc := range []struct {
		name   string
		dist   int
		length int
	}{
		{name: "non-overlap-64k", dist: 64 << 10, length: 64 << 10},
		{name: "overlap-dist-1-64k", dist: 1, length: 64 << 10},
		{name: "overlap-dist-32-64k", dist: 32, length: 64 << 10},
		{name: "short-dist-8-len-3", dist: 8, length: 3},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(tc.length))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				if err := copyBackRef(dst, outputPos, tc.dist, tc.length); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkCopyLiteralRun(b *testing.B) {
	src := benchmarkRandomBytes(64 << 10)
	dst := make([]byte, len(src))

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		inPos, outPos := 0, 0
		if err := copyLiteralRun(src, &inPos, dst, &outPos, len(src)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCountEqualBytes(b *testing.B) {
	var buffer [hcBufferGuardSize]byte
	copy(buffer[:], benchmarkRandomBytes(len(buffer)))
	const (
		left  = 0
		right = hcMaxMatchLen
	)

	for _, length := range []int{8, 64, hcMaxMatchLen} {
		copy(buffer[right:right+length], buffer[left:left+length])
		buffer[right+length] ^= 1
		b.Run(fmt.Sprintf("match-%d", length), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(length))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_ = countEqualBytes(&buffer, left, right, 2, left+length+1)
			}
		})
	}
}
