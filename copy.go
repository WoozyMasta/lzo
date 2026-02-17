// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/lzo

package lzo

// copyBackRef copies length bytes from dst[outputPos-dist:outputPos-dist+length] to dst[outputPos:outputPos+length].
// If distance < length, source and destination overlap; copy must be byte-by-byte so that
// repeated bytes (RLE) are correct. The built-in copy does not handle overlapping regions
// where src precedes dst.
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

	for i := range length {
		dst[outputPos+i] = dst[mPos+i]
	}

	return nil
}
