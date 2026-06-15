// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/lzo

package lzo

// copyBackRefUnchecked expands a back-reference after bounds have been validated.
// If dist < length, LZ semantics require forward expansion,
// where newly written bytes become valid source for the remainder of the match.
func copyBackRefUnchecked(dst []byte, outputPos, matchPos, dist, length int) {
	if dist >= length {
		copy(dst[outputPos:outputPos+length], dst[matchPos:matchPos+length])
		return
	}

	// Seed with one original distance chunk.
	copy(dst[outputPos:outputPos+dist], dst[matchPos:outputPos])
	copied := dist

	// Grow copied region exponentially, which is much cheaper than byte-by-byte loops.
	for copied < length {
		n := copy(dst[outputPos+copied:outputPos+length], dst[outputPos:outputPos+copied])
		copied += n
	}
}
