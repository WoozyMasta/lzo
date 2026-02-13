// SPDX-License-Identifier: GPL-2.0-only
// Source: github.com/woozymasta/lzo

package lzo

import (
	"bytes"
	"fmt"
	"testing"
)

func benchmarkInputSets() map[string][]byte {
	return map[string][]byte{
		"small-text-4k":   bytes.Repeat([]byte("lzo benchmark text payload "), 160),
		"pattern-128k":    bytes.Repeat([]byte("ABCDEF0123456789"), 8192),
		"byte-cycle-256k": bytes.Repeat([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, 26214),
	}
}

func BenchmarkCompress(b *testing.B) {
	levels := []int{1, 5, 9}
	for inputName, inputData := range benchmarkInputSets() {
		for _, level := range levels {
			name := fmt.Sprintf("%s/level-%d", inputName, level)
			b.Run(name, func(b *testing.B) {
				opts := &CompressOptions{Level: level}
				b.ReportAllocs()
				b.SetBytes(int64(len(inputData)))
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					_, err := Compress(inputData, opts)
					if err != nil {
						b.Fatalf("Compress failed: %v", err)
					}
				}
			})
		}
	}
}

func BenchmarkDecompress(b *testing.B) {
	levels := []int{1, 5, 9}
	for inputName, inputData := range benchmarkInputSets() {
		for _, level := range levels {
			compressedData, err := Compress(inputData, &CompressOptions{Level: level})
			if err != nil {
				b.Fatalf("setup Compress failed for %s level %d: %v", inputName, level, err)
			}

			opts := DefaultDecompressOptions(len(inputData))
			if _, err := Decompress(compressedData, opts); err != nil {
				b.Fatalf("setup Decompress failed for %s level %d: %v", inputName, level, err)
			}

			name := fmt.Sprintf("%s/from-level-%d", inputName, level)
			b.Run(name, func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(inputData)))
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
