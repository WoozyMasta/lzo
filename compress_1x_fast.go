// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/lzo

package lzo

// compress1xFastCore performs the fast LZO1X-1 parse and returns pending literal tail.
func compress1xFastCore(in []byte) (out []byte, literalTailSize int) {
	inputLen := len(in)
	inputLimit := inputLen - maxLenM2 - 5
	dict := make([]int32, 1<<dictBits)
	literalStart := 0
	inputPos := 4

	for {
		// Hash the next 4-byte sequence into the dictionary.
		key := int(in[inputPos+3])
		key = (key << 6) ^ int(in[inputPos+2])
		key = (key << 5) ^ int(in[inputPos+1])
		key = (key << 5) ^ int(in[inputPos+0])
		dictIndex := ((0x21 * key) >> 5) & dictMask

		matched := false

		// Probe two related hash slots to improve hit rate without extra structures.
		for attempt := range 2 {
			matchPos, matchOffset := findFastCandidate(dict, in, inputPos, dictIndex)
			tryMatch := matchPos >= 0 && (matchOffset <= maxOffsetM2 || in[matchPos+3] == in[inputPos+3])

			if tryMatch &&
				in[matchPos] == in[inputPos] &&
				in[matchPos+1] == in[inputPos+1] &&
				in[matchPos+2] == in[inputPos+2] {
				dict[dictIndex] = int32(inputPos + 1) //nolint:gosec // G115: input position fits int32 for LZO input sizes

				if inputPos != literalStart {
					out = appendFastLiteral(out, in[literalStart:inputPos])
					literalStart = inputPos
				}

				var i int
				inputPos += 3

				// Fast short extension for the first bytes; this is the hot path.
				for i = 3; i < 9; i++ {
					inputPos++

					if in[matchPos+i] != in[inputPos-1] {
						break
					}
				}

				if i < 9 {
					inputPos--
					matchLen := inputPos - literalStart

					switch { // Pick the shortest opcode class that can represent this match.
					case matchOffset <= maxOffsetM2:
						matchOffset--
						out = append(out,
							opcodeByte(((matchLen-1)<<5)|((matchOffset&7)<<2)),
							opcodeByte(matchOffset>>3),
						)

					case matchOffset <= maxOffsetM3:
						matchOffset--
						out = append(out,
							opcodeByte(markerM3|(matchLen-2)),
							opcodeByte((matchOffset&63)<<2),
							opcodeByte(matchOffset>>6),
						)

					default:
						matchOffset -= 0x4000
						out = append(out,
							opcodeByte(markerM4|((matchOffset&0x4000)>>11)|(matchLen-2)),
							opcodeByte((matchOffset&63)<<2),
							opcodeByte(matchOffset>>6),
						)
					}
				} else {
					// Slow path for long matches beyond the initial short extension window.
					m := matchPos + maxLenM2 + 1
					for inputPos < inputLen && in[m] == in[inputPos] {
						m++
						inputPos++
					}

					matchLen := inputPos - literalStart
					if matchOffset <= maxOffsetM3 {
						matchOffset--
						if matchLen <= 33 {
							out = append(out, opcodeByte(markerM3|(matchLen-2)))
						} else {
							matchLen -= 33
							out = append(out, opcodeByte(markerM3))
							out = appendFastMultiple(out, matchLen)
						}
					} else {
						matchOffset -= 0x4000
						if matchLen <= maxLenM4 {
							out = append(out, opcodeByte(markerM4|((matchOffset&0x4000)>>11)|(matchLen-2)))
						} else {
							matchLen -= maxLenM4
							out = append(out, opcodeByte(markerM4|((matchOffset&0x4000)>>11)))
							out = appendFastMultiple(out, matchLen)
						}
					}
					out = append(out, opcodeByte((matchOffset&63)<<2), opcodeByte(matchOffset>>6))
				}

				// Next literal run, if any, starts after the emitted match.
				literalStart = inputPos
				matched = true
				break
			}

			if attempt == 0 {
				dictIndex = (dictIndex & (dictMask & 0x7ff)) ^ (dictHigh | 0x1f)
			}
		}

		if matched {
			if inputPos >= inputLimit {
				break
			}

			continue
		}

		// Literal step with lazy skip, standard for the LZO1X-1 fast parser.
		dict[dictIndex] = int32(inputPos + 1) //nolint:gosec // G115: input position fits int32 for LZO input sizes
		inputPos += 1 + (inputPos-literalStart)>>5
		if inputPos >= inputLimit {
			break
		}
	}

	literalTailSize = inputLen - literalStart
	return
}

// compress1xFast is the fast LZO1X-1 compressor (level 0 or 1).
func compress1xFast(in []byte) []byte {
	var literalTailSize int
	inLen := len(in)

	var out []byte
	if inLen <= maxLenM2+5 {
		literalTailSize = inLen
	} else {
		out, literalTailSize = compress1xFastCore(in)
	}

	if literalTailSize > 0 {
		ii := inLen - literalTailSize
		out = appendFastLiteral(out, in[ii:ii+literalTailSize])
	}

	out = append(out, markerM4|1, 0, 0)
	return out
}

// findFastCandidate returns (matchPos, matchOffset) for the given dict slot, or (-1, 0) if none.
func findFastCandidate(dict []int32, in []byte, inputPos, dictIndex int) (matchPos int, matchOffset int) {
	matchPos = int(dict[dictIndex]) - 1
	if matchPos < 0 {
		return -1, 0
	}

	if inputPos == matchPos || (inputPos-matchPos) > maxOffsetM4 {
		return -1, 0
	}

	matchOffset = inputPos - matchPos
	if matchOffset <= maxOffsetM2 || in[matchPos+3] == in[inputPos+3] {
		return matchPos, matchOffset
	}

	return -1, 0
}

// appendFastLiteral appends a literal run and its header encoding.
// lit must be non-empty.
func appendFastLiteral(out []byte, lit []byte) []byte {
	if len(lit) == 0 {
		return out
	}
	literalCount := len(lit)

	switch {
	case len(out) == 0 && literalCount <= 238:
		out = append(out, opcodeByte(17+literalCount))
	case literalCount <= 3:
		out[len(out)-2] |= opcodeByte(literalCount)
	case literalCount <= 18:
		out = append(out, opcodeByte(literalCount-3))
	default:
		out = append(out, 0)
		out = appendFastMultiple(out, literalCount-18)
	}

	out = append(out, lit...)
	return out
}

// appendFastMultiple appends a multiple of 255 to the output.
func appendFastMultiple(out []byte, t int) []byte {
	for t > 255 {
		out = append(out, 0)
		t -= 255
	}

	out = append(out, opcodeByte(t))
	return out
}
