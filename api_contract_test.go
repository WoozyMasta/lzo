package lzo

import (
	"bytes"
	"testing"
)

func TestAPIContract_DecompressAllowsTrailingBytes(t *testing.T) {
	src := bytes.Repeat([]byte("api-contract"), 64)

	compressed, err := Compress(src, &CompressOptions{Level: 5})
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	payload := append(append([]byte{}, compressed...), []byte("tail")...)
	out, err := Decompress(payload, DefaultDecompressOptions(len(src)))
	if err != nil {
		t.Fatalf("Decompress with trailing bytes failed: %v", err)
	}

	if !bytes.Equal(out, src) {
		t.Fatal("decoded output mismatch for trailing-byte input")
	}
}

func TestAPIContract_DecompressCanReturnShorterThanOutLen(t *testing.T) {
	src := bytes.Repeat([]byte("short-output"), 32)

	compressed, err := Compress(src, nil)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	out, err := Decompress(compressed, DefaultDecompressOptions(len(src)+256))
	if err != nil {
		t.Fatalf("Decompress failed: %v", err)
	}

	if len(out) != len(src) {
		t.Fatalf("decoded length mismatch: got=%d want=%d", len(out), len(src))
	}

	if !bytes.Equal(out, src) {
		t.Fatal("decoded output mismatch")
	}
}

func TestAPIContract_DecompressCanonicalStream(t *testing.T) {
	// This stream is used as a canonical example in lzokay-rs docs:
	// it expands to 512 zero bytes.
	compressed := []byte{0x12, 0x00, 0x20, 0x00, 0xdf, 0x00, 0x00, 0x11, 0x00, 0x00}
	expected := make([]byte, 512)

	out, err := Decompress(compressed, DefaultDecompressOptions(512))
	if err != nil {
		t.Fatalf("Decompress failed for canonical stream: %v", err)
	}

	if !bytes.Equal(out, expected) {
		t.Fatal("canonical stream decoded data mismatch")
	}
}
