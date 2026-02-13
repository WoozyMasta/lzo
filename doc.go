// SPDX-License-Identifier: GPL-2.0-only
// Source: github.com/woozymasta/lzo

/*
Package lzo implements LZO1X compression and decompression (lzo1x_decompress_safe–compatible).

The format uses match types M1–M4 with different offset and length bounds; the
stream ends with a terminator (distance 0x4000, length 1). Suitable for archives
and binary formats that use LZO1X.

# Decompress

OutLen is required (use DecompressOptions). From a byte slice:

	out, err := lzo.Decompress(compressed, lzo.DefaultDecompressOptions(expectedLen))

To get the number of input bytes consumed (e.g. for back-to-back compressed blocks):

	out, nRead, err := lzo.DecompressN(compressed, lzo.DefaultDecompressOptions(expectedLen))
	// advance: compressed = compressed[nRead:]

From an io.Reader (e.g. stream with known decompressed size):

	out, err := lzo.DecompressFromReader(r, lzo.DefaultDecompressOptions(expectedLen))

# Compress

Options may be nil (default level 1). Level 0 or 1 = fast LZO1X-1; 2–9 = LZO1X-999:

	out, err := lzo.Compress(data, nil)
	out, err := lzo.Compress(data, &lzo.CompressOptions{Level: 9})
*/
package lzo
