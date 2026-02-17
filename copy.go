// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/lzo

package lzo

// copyBackRef copies length bytes from dst[outputPos-dist:outputPos-dist+length] to dst[outputPos:outputPos+length].
// If dist < length, LZ semantics require "forward" expansion (newly written bytes become
// valid source for the remainder of the match). We implement this using repeated doubling:
// first copy one full distance chunk, then copy from already-expanded output.
func copyBackRef(dst []byte, outputPos, dist, length int) error {
	mPos := outputPos - dist
	if mPos < 0 {
		return ErrLookBehindUnderrun
	}

	if outputPos+length > len(dst) {
		return ErrOutputOverrun
	}

	if dist >= length {
		copy(dst[outputPos:outputPos+length], dst[mPos:mPos+length])
		return nil
	}

	// Seed with one original distance chunk.
	copy(dst[outputPos:outputPos+dist], dst[mPos:outputPos])
	copied := dist

	// Grow copied region exponentially, which is much cheaper than byte-by-byte loops.
	for copied < length {
		n := copy(dst[outputPos+copied:outputPos+length], dst[outputPos:outputPos+copied])
		copied += n
	}

	return nil
}
