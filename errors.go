// SPDX-License-Identifier: GPL-2.0-only
// Source: github.com/woozymasta/lzo

package lzo

import "errors"

// Sentinel errors for decompression and compression.
var (
	// ErrEmptyInput is returned when the input slice or stream is empty.
	ErrEmptyInput = errors.New("empty input")
	// ErrInputOverrun is returned when the decoder reads past the end of input.
	ErrInputOverrun = errors.New("input overrun")
	// ErrOutputOverrun is returned when the decoder would write past the output buffer.
	ErrOutputOverrun = errors.New("output overrun")
	// ErrLookBehindUnderrun is returned when a back-reference points before the start of the output.
	ErrLookBehindUnderrun = errors.New("lookbehind underrun")
	// ErrUnexpectedEOF is returned when the stream ends before the terminator or expected size.
	ErrUnexpectedEOF = errors.New("unexpected end of input")
	// ErrOptionsRequired is returned when Decompress is called with nil options (OutLen is required).
	ErrOptionsRequired = errors.New("options required: OutLen must be set")
	// ErrInputTooLarge is returned when DecompressFromReader reads more than MaxInputSize bytes.
	ErrInputTooLarge = errors.New("input exceeds MaxInputSize")

	// ErrCompressInternal is returned when the compressor hits an internal invariant violation
	// (e.g. invalid match state, invalid window state). Callers can use errors.Is(err, lzo.ErrCompressInternal).
	ErrCompressInternal = errors.New("internal compressor error")
)
