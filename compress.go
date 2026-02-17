// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/lzo

package lzo

// Compress compresses src with LZO1X. opts may be nil (uses default level 1).
// Level 0 or 1 = fast LZO1X-1; 2–9 = LZO1X-999 (better ratio, slower).
func Compress(src []byte, opts *CompressOptions) ([]byte, error) {
	if opts == nil {
		opts = DefaultCompressOptions()
	}
	level := opts.Level
	level = max(level, 0)

	if level <= 1 {
		return compress1xFast(src), nil
	}

	level = min(level, 9)
	return compress999Level(src, level)
}
