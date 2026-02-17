package lzo

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCompatibility_LzokayNativeCorpus(t *testing.T) {
	compressedDir := filepath.Join("ref", "lzokay-native-rs", "test-data", "compressed")
	uncompressedDir := filepath.Join("ref", "lzokay-native-rs", "test-data", "uncompressed")

	if _, err := os.Stat(compressedDir); err != nil {
		t.Skipf("compat corpus not found: %v", err)
	}

	entries, err := os.ReadDir(compressedDir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", compressedDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if filepath.Ext(name) != ".lzo" {
			continue
		}

		testName := name
		t.Run(testName, func(t *testing.T) {
			compressedPath := filepath.Join(compressedDir, testName)
			compressedData, err := os.ReadFile(compressedPath)
			if err != nil {
				t.Fatalf("ReadFile(%q): %v", compressedPath, err)
			}

			baseName := testName[:len(testName)-len(".lzo")]
			plainPath := filepath.Join(uncompressedDir, baseName)
			plainData, err := os.ReadFile(plainPath)
			if err != nil {
				t.Fatalf("ReadFile(%q): %v", plainPath, err)
			}

			out, err := Decompress(compressedData, DefaultDecompressOptions(len(plainData)))
			if err != nil {
				t.Fatalf("Decompress(%q): %v", testName, err)
			}

			if !bytes.Equal(out, plainData) {
				t.Fatalf("decoded mismatch for %q: got=%d want=%d", testName, len(out), len(plainData))
			}
		})
	}
}
