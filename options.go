// SPDX-License-Identifier: GPL-2.0-only
// Source: github.com/woozymasta/lzo

package lzo

// DecompressOptions configures decompression.
// OutLen is required (expected decompressed size); MaxInputSize limits reads when using DecompressFromReader.
type DecompressOptions struct {
	// OutLen is the expected decompressed size (required for buffer allocation and safety).
	OutLen int
	// MaxInputSize limits how many bytes DecompressFromReader may read (0 = no limit).
	MaxInputSize int
}

// DefaultDecompressOptions returns options with the given output length and no input limit.
func DefaultDecompressOptions(outLen int) *DecompressOptions {
	return &DecompressOptions{OutLen: outLen}
}

// CompressOptions configures compression (LZO1X-1 fast vs LZO1X-999 levels).
type CompressOptions struct {
	// Level: 0 or 1 = fast LZO1X-1; 2–9 = LZO1X-999 (higher = better ratio, slower).
	Level int
}

// DefaultCompressOptions returns options for fast compression (level 1).
func DefaultCompressOptions() *CompressOptions {
	return &CompressOptions{Level: 1}
}
