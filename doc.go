// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/lzo

/*
Package lzo implements LZO1X compression and decompression (lzo1x_decompress_safe–compatible).

The format uses match types M1–M4 with different offset and length bounds; the
stream ends with the standard terminator bytes `0x11 0x00 0x00`. Suitable for archives
and binary formats that use LZO1X.

The current encoder/decoder cores are MIT-licensed and implemented from permissive
references. Main implementation reference: AxioDL/lzokay (MIT).

# Decompress

OutLen is required (use DecompressOptions). From a byte slice:

	out, err := lzo.Decompress(compressed, lzo.DefaultDecompressOptions(expectedLen))

To get the number of input bytes consumed (e.g. for back-to-back compressed blocks):

	out, nRead, err := lzo.DecompressN(compressed, lzo.DefaultDecompressOptions(expectedLen))
	// advance: compressed = compressed[nRead:]

To reuse caller-managed output memory (no per-call output allocation):

	dst := make([]byte, expectedLen)
	out, err := lzo.DecompressInto(compressed, dst)
	out, nRead, err := lzo.DecompressNInto(compressed, dst)

From an io.Reader (e.g. stream with known decompressed size):

	out, err := lzo.DecompressFromReader(r, lzo.DefaultDecompressOptions(expectedLen))

# Compress

Options may be nil (default level 1). Level 0 or 1 = fast LZO1X-1; 2–9 = LZO1X-999:

	out, err := lzo.Compress(data, nil)
	out, err := lzo.Compress(data, &lzo.CompressOptions{Level: 9})
*/
package lzo
