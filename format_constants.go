// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/lzo

package lzo

// LZO1X format constants: M1/M2/M3/M4 offset and length bounds, and dictionary parameters.

// Match offset bounds (max distance for each match type).
const (
	maxOffsetM1 = 0x0400
	maxOffsetM2 = 0x0800
	maxOffsetM3 = 0x4000
	maxOffsetM4 = 0xbfff
	maxOffsetMX = maxOffsetM1 + maxOffsetM2
)

// Match length bounds per type.
const (
	minLenM2 = 3
	maxLenM2 = 8
	maxLenM3 = 33
	maxLenM4 = 9
)

// Instruction byte markers for match types.
const (
	markerM1 = 0
	markerM2 = 64
	markerM3 = 32
	markerM4 = 16
)

// Dictionary hash parameters used by the compressor.
const (
	dictBits = 14                  // number of bits in the dictionary hash
	dictMask = (1 << dictBits) - 1 // mask for the dictionary hash
	dictHigh = (dictMask >> 1) + 1 // high bit for the dictionary hash
)
