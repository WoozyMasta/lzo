package compat

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/woozymasta/lzo"
)

func TestLibLZO2Compatibility(t *testing.T) {
	helper := os.Getenv("LZO2_HELPER")
	if helper == "" {
		t.Fatal("LZO2_HELPER is not set")
	}

	for _, input := range libLZO2CompatibilityInputs(t) {
		t.Run(input.name, func(t *testing.T) {
			inputPath := filepath.Join(t.TempDir(), "input")
			if err := os.WriteFile(inputPath, input.data, 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			for _, level := range []int{1, 9} {
				t.Run(fmt.Sprintf("ours-to-liblzo2/level-%d", level), func(t *testing.T) {
					compressed, err := lzo.Compress(input.data, &lzo.CompressOptions{Level: level})
					if err != nil {
						t.Fatalf("Compress: %v", err)
					}

					compressedPath := filepath.Join(t.TempDir(), "input.lzo")
					if err := os.WriteFile(compressedPath, compressed, 0o600); err != nil {
						t.Fatalf("WriteFile: %v", err)
					}

					decoded, err := runLZO2Helper(helper, "decompress", compressedPath, strconv.Itoa(len(input.data)))
					if err != nil {
						t.Fatalf("liblzo2 decompress: %v", err)
					}
					if !bytes.Equal(decoded, input.data) {
						t.Fatalf("decoded output mismatch at byte %d", firstMismatch(decoded, input.data))
					}
				})
			}

			for _, highCompression := range []bool{false, true} {
				name := "fast"
				if highCompression {
					name = "high"
				}
				t.Run("liblzo2-to-ours/"+name, func(t *testing.T) {
					operation := "compress-fast"
					if highCompression {
						operation = "compress-high"
					}
					compressed, err := runLZO2Helper(helper, operation, inputPath)
					if err != nil {
						t.Fatalf("liblzo2 compress: %v", err)
					}

					decoded, err := lzo.Decompress(compressed, lzo.DefaultDecompressOptions(len(input.data)))
					if err != nil {
						t.Fatalf("Decompress: %v", err)
					}
					if !bytes.Equal(decoded, input.data) {
						t.Fatalf("decoded output mismatch at byte %d", firstMismatch(decoded, input.data))
					}
				})
			}
		})
	}
}

func runLZO2Helper(helper string, args ...string) ([]byte, error) {
	cmd := exec.Command(helper, args...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("%s: %w: %s", helper, err, exitErr.Stderr)
		}
		return nil, fmt.Errorf("%s: %w", helper, err)
	}
	return output, nil
}

func libLZO2CompatibilityInputs(t *testing.T) []struct {
	name string
	data []byte
} {
	t.Helper()

	corpusDir := filepath.Join("..", "corpus")
	entries, err := os.ReadDir(corpusDir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", corpusDir, err)
	}

	inputs := make([]struct {
		name string
		data []byte
	}, 0, len(entries)+3)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".txt" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(corpusDir, entry.Name()))
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", entry.Name(), err)
		}
		inputs = append(inputs, struct {
			name string
			data []byte
		}{name: entry.Name(), data: data})
	}

	inputs = append(inputs,
		struct {
			name string
			data []byte
		}{name: "generated-random-256k", data: randomBytes(256 << 10)},
		struct {
			name string
			data []byte
		}{name: "generated-mixed-256k", data: mixedBytes(256 << 10)},
		struct {
			name string
			data []byte
		}{name: "generated-byte-cycle-256k", data: bytes.Repeat([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, 26214)},
	)

	return inputs
}

func randomBytes(size int) []byte {
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

func mixedBytes(size int) []byte {
	data := randomBytes(size)
	pattern := []byte("level=info component=lzo message=compat payload=0123456789abcdef\n")
	for start := 0; start < len(data); start += 4096 {
		end := min(start+3072, len(data))
		for pos := start; pos < end; pos += len(pattern) {
			copy(data[pos:end], pattern)
		}
	}
	return data
}

func firstMismatch(left, right []byte) int {
	limit := min(len(left), len(right))
	for i := 0; i < limit; i++ {
		if left[i] != right[i] {
			return i
		}
	}
	return limit
}
