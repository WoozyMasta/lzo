package lzo

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestDecompress_OptionsRequired(t *testing.T) {
	_, err := Decompress([]byte{0x11, 0x00}, nil)
	if !errors.Is(err, ErrOptionsRequired) {
		t.Fatalf("expected ErrOptionsRequired, got %v", err)
	}

	_, err = DecompressFromReader(strings.NewReader("\x00"), nil)
	if !errors.Is(err, ErrOptionsRequired) {
		t.Fatalf("expected ErrOptionsRequired (reader), got %v", err)
	}
}

func TestDecompress_EmptyInput(t *testing.T) {
	_, err := Decompress(nil, DefaultDecompressOptions(0))
	if !errors.Is(err, ErrEmptyInput) {
		t.Fatalf("expected ErrEmptyInput, got %v", err)
	}
}

func TestDecompress_TruncatedInputAlwaysFails(t *testing.T) {
	data := bytes.Repeat([]byte("0123456789abcdef"), 256)
	cmp, err := Compress(data, &CompressOptions{Level: 9})
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}
	if len(cmp) < 4 {
		t.Fatalf("compressed data unexpectedly short: %d", len(cmp))
	}

	maxCut := min(32, len(cmp)-1)
	for cut := 1; cut <= maxCut; cut++ {
		truncated := cmp[:len(cmp)-cut]
		_, decErr := Decompress(truncated, DefaultDecompressOptions(len(data)))
		if decErr == nil {
			t.Fatalf("expected error for cut=%d", cut)
		}
	}
}

func TestDecompress_OutLenTooSmall(t *testing.T) {
	data := bytes.Repeat([]byte("AABBCCDDEEFF"), 512)
	cmp, err := Compress(data, &CompressOptions{Level: 5})
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	_, err = Decompress(cmp, DefaultDecompressOptions(len(data)-1))
	if err == nil {
		t.Fatal("expected decompression error with too small OutLen")
	}
	if !errors.Is(err, ErrInputOverrun) && !errors.Is(err, ErrOutputOverrun) {
		t.Fatalf("unexpected error for too small OutLen: %v", err)
	}
}

func TestDecompressFromReader_MaxInputSize(t *testing.T) {
	data := bytes.Repeat([]byte("xyz"), 200)
	cmp, err := Compress(data, nil)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	opts := DefaultDecompressOptions(len(data))
	opts.MaxInputSize = len(cmp) - 1
	_, err = DecompressFromReader(bytes.NewReader(cmp), opts)
	if !errors.Is(err, ErrInputTooLarge) {
		t.Fatalf("expected ErrInputTooLarge, got %v", err)
	}
}

func TestDecompressN_ReturnsConsumedBytes(t *testing.T) {
	data := bytes.Repeat([]byte("0123456789"), 100)
	cmp, err := Compress(data, nil)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	decoded, nRead, err := DecompressN(cmp, DefaultDecompressOptions(len(data)))
	if err != nil {
		t.Fatalf("DecompressN failed: %v", err)
	}

	if nRead != len(cmp) {
		t.Errorf("nRead = %d, want %d (full compressed length)", nRead, len(cmp))
	}
	if !bytes.Equal(decoded, data) {
		t.Errorf("decoded mismatch")
	}

	// Back-to-back: extra bytes after the block should not be consumed
	extra := []byte("trailing")
	src := append(append([]byte(nil), cmp...), extra...)
	decoded2, nRead2, err := DecompressN(src, DefaultDecompressOptions(len(data)))
	if err != nil {
		t.Fatalf("DecompressN with trailing failed: %v", err)
	}
	if nRead2 != len(cmp) {
		t.Errorf("nRead with trailing = %d, want %d", nRead2, len(cmp))
	}
	if !bytes.Equal(decoded2, data) {
		t.Errorf("decoded with trailing mismatch")
	}
	if nRead2 < len(src) && !bytes.Equal(src[nRead2:], extra) {
		t.Errorf("advancing by nRead should leave trailing bytes unchanged")
	}
}

func TestDecompressInto_ReusesCallerBuffer(t *testing.T) {
	data := bytes.Repeat([]byte("decode-into"), 256)
	cmp, err := Compress(data, &CompressOptions{Level: 5})
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	dst := make([]byte, len(data))
	out, err := DecompressInto(cmp, dst)
	if err != nil {
		t.Fatalf("DecompressInto failed: %v", err)
	}

	if len(out) != len(data) {
		t.Fatalf("decoded length mismatch: got=%d want=%d", len(out), len(data))
	}
	if !bytes.Equal(out, data) {
		t.Fatal("decoded output mismatch")
	}
	if len(out) > 0 && &out[0] != &dst[0] {
		t.Fatal("DecompressInto should return a slice over provided destination buffer")
	}
}

func TestDecompressNInto_ReturnsConsumedBytes(t *testing.T) {
	data := bytes.Repeat([]byte("concat-block"), 180)
	cmp, err := Compress(data, &CompressOptions{Level: 9})
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	src := append(append([]byte(nil), cmp...), []byte("tail")...)
	dst := make([]byte, len(data))

	out, nRead, err := DecompressNInto(src, dst)
	if err != nil {
		t.Fatalf("DecompressNInto failed: %v", err)
	}

	if nRead != len(cmp) {
		t.Fatalf("nRead mismatch: got=%d want=%d", nRead, len(cmp))
	}
	if !bytes.Equal(out, data) {
		t.Fatal("decoded output mismatch")
	}
}

func TestDecompressInto_BufferTooSmall(t *testing.T) {
	data := bytes.Repeat([]byte("small-buffer"), 128)
	cmp, err := Compress(data, &CompressOptions{Level: 5})
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	_, err = DecompressInto(cmp, make([]byte, len(data)-1))
	if !errors.Is(err, ErrOutputOverrun) {
		t.Fatalf("expected ErrOutputOverrun, got %v", err)
	}
}

func TestCopyBackRef(t *testing.T) {
	t.Run("non-overlapping", func(t *testing.T) {
		dst := []byte("abcdefghXXXXXXXX")
		if err := copyBackRef(dst, 8, 8, 4); err != nil {
			t.Fatalf("copyBackRef failed: %v", err)
		}
		if got, want := string(dst), "abcdefghabcdXXXX"; got != want {
			t.Fatalf("unexpected dst: got %q want %q", got, want)
		}
	})

	t.Run("overlapping", func(t *testing.T) {
		dst := []byte{'A', 'B', 'C', 0, 0, 0, 0, 0}
		if err := copyBackRef(dst, 3, 3, 5); err != nil {
			t.Fatalf("copyBackRef failed: %v", err)
		}
		if got, want := string(dst), "ABCABCAB"; got != want {
			t.Fatalf("unexpected dst: got %q want %q", got, want)
		}
	})

	t.Run("lookbehind-underrun", func(t *testing.T) {
		dst := make([]byte, 8)
		err := copyBackRef(dst, 2, 3, 2)
		if !errors.Is(err, ErrLookBehindUnderrun) {
			t.Fatalf("expected ErrLookBehindUnderrun, got %v", err)
		}
	})

	t.Run("output-overrun", func(t *testing.T) {
		dst := make([]byte, 8)
		err := copyBackRef(dst, 7, 1, 2)
		if !errors.Is(err, ErrOutputOverrun) {
			t.Fatalf("expected ErrOutputOverrun, got %v", err)
		}
	})
}
