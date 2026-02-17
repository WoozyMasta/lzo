// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/lzo

package lzo

// opcodeByte packs an opcode fragment to one byte as required by LZO bit layout.
// Callers pass values whose low 8 bits are the serialized representation.
func opcodeByte(v int) byte {
	// #nosec G115 -- LZO opcodes intentionally encode only low 8 bits.
	return byte(v & 0xff)
}
