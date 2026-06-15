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
	out, err := lzo.DecompressFromReaderInto(r, dst, lzo.DefaultDecompressOptions(expectedLen))

Reader APIs read the complete compressed stream before decoding.
DecompressOptions.MaxInputSize bounds the number of compressed bytes read.

# Compress

Options may be nil (default level 1). Level 0 or 1 = fast LZO1X-1; 2–9 = LZO1X-999:

	out, err := lzo.Compress(data, nil)
	out, err := lzo.Compress(data, &lzo.CompressOptions{Level: 9})

To reuse caller-managed output memory:

	dst := make([]byte, lzo.MaxCompressedSize(len(data)))
	out, err := lzo.CompressInto(data, dst, nil)
	out, err := lzo.AppendCompress(dst[:0], data, nil)

To retain LZO1X-999 state across calls without relying on a shared pool:

	encoder := lzo.NewEncoder()
	out, err := encoder.CompressInto(data, dst, &lzo.CompressOptions{Level: 9})

Each Encoder retains one LZO1X-999 dictionary. It must not be copied after
first use or used concurrently.
*/
package lzo
