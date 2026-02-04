package lzo

// Compress compresses src with LZO1X. opts may be nil (uses default level 1).
// Level 0 or 1 = fast LZO1X-1; 2–9 = LZO1X-999 (better ratio, slower).
func Compress(src []byte, opts *CompressOptions) ([]byte, error) {
	if opts == nil {
		opts = DefaultCompressOptions()
	}
	level := opts.Level
	if level < 0 {
		level = 0
	}

	if level <= 1 {
		return compress1xFast(src), nil
	}
	if level > 9 {
		level = 9
	}

	return compress9x(src, fixedLevels[level-1])
}

func compress(in []byte) (out []byte, literalTailSize int) {
	inputLen := len(in)
	inputLimit := inputLen - maxLenM2 - 5
	dict := make([]int32, 1<<dictBits)
	literalStart := 0
	inputPos := 4
	for {
		key := int(in[inputPos+3])
		key = (key << 6) ^ int(in[inputPos+2])
		key = (key << 5) ^ int(in[inputPos+1])
		key = (key << 5) ^ int(in[inputPos+0])
		dictIndex := ((0x21 * key) >> 5) & dictMask

		matchPos, matchOffset := findCandidate(dict, in, inputPos, dictIndex)
		tryMatch := matchPos >= 0 && (matchOffset <= maxOffsetM2 || in[matchPos+3] == in[inputPos+3])
		if tryMatch && in[matchPos] == in[inputPos] && in[matchPos+1] == in[inputPos+1] && in[matchPos+2] == in[inputPos+2] {
			// emit match
			dict[dictIndex] = int32(inputPos + 1) //nolint:gosec // G115: input position fits int32 for LZO input sizes
			if inputPos != literalStart {
				literalCount := inputPos - literalStart
				if len(out) == 0 && literalCount <= 238 {
					out = append(out, byte(17+literalCount))
				} else if literalCount <= 3 {
					out[len(out)-2] |= byte(literalCount)
				} else if literalCount <= 18 {
					out = append(out, byte(literalCount-3))
				} else {
					out = append(out, 0)
					out = appendMultiple(out, literalCount-18)
				}
				out = append(out, in[literalStart:literalStart+literalCount]...)
				literalStart += literalCount
			}

			// find match
			var i int
			inputPos += 3
			for i = 3; i < 9; i++ {
				inputPos++
				if in[matchPos+i] != in[inputPos-1] {
					break
				}
			}

			if i < 9 {
				inputPos--
				matchLen := inputPos - literalStart
				if matchOffset <= maxOffsetM2 {
					matchOffset--
					out = append(out,
						byte((((matchLen - 1) << 5) | ((matchOffset & 7) << 2))),
						byte((matchOffset >> 3)))
				} else if matchOffset <= maxOffsetM3 {
					matchOffset--
					out = append(out,
						byte(markerM3|(matchLen-2)),
						byte((matchOffset&63)<<2),
						byte(matchOffset>>6))
				} else {
					matchOffset -= 0x4000
					out = append(out,
						byte(markerM4|((matchOffset&0x4000)>>11)|(matchLen-2)),
						byte((matchOffset&63)<<2),
						byte(matchOffset>>6))
				}
			} else {
				m := matchPos + maxLenM2 + 1
				for inputPos < inputLen && in[m] == in[inputPos] {
					m++
					inputPos++
				}
				matchLen := inputPos - literalStart
				if matchOffset <= maxOffsetM3 {
					matchOffset--
					if matchLen <= 33 {
						out = append(out, byte(markerM3|(matchLen-2)))
					} else {
						matchLen -= 33
						out = append(out, byte(markerM3))
						out = appendMultiple(out, matchLen)
					}
				} else {
					matchOffset -= 0x4000
					if matchLen <= maxLenM4 {
						out = append(out, byte(markerM4|((matchOffset&0x4000)>>11)|(matchLen-2)))
					} else {
						matchLen -= maxLenM4
						out = append(out, byte(markerM4|((matchOffset&0x4000)>>11)))
						out = appendMultiple(out, matchLen)
					}
				}
				out = append(out, byte((matchOffset&63)<<2), byte(matchOffset>>6))
			}

			literalStart = inputPos
			if inputPos >= inputLimit {
				break
			}

			continue
		}

		dictIndex = (dictIndex & (dictMask & 0x7ff)) ^ (dictHigh | 0x1f)
		matchPos, matchOffset = findCandidate(dict, in, inputPos, dictIndex)
		tryMatch = matchPos >= 0 && (matchOffset <= maxOffsetM2 || in[matchPos+3] == in[inputPos+3])
		if tryMatch && in[matchPos] == in[inputPos] && in[matchPos+1] == in[inputPos+1] && in[matchPos+2] == in[inputPos+2] {
			dict[dictIndex] = int32(inputPos + 1) //nolint:gosec // G115: input position fits int32 for LZO input sizes

			// emit literal
			if inputPos != literalStart {
				literalCount := inputPos - literalStart
				if len(out) == 0 && literalCount <= 238 {
					out = append(out, byte(17+literalCount))
				} else if literalCount <= 3 {
					out[len(out)-2] |= byte(literalCount)
				} else if literalCount <= 18 {
					out = append(out, byte(literalCount-3))
				} else {
					out = append(out, 0)
					out = appendMultiple(out, literalCount-18)
				}
				out = append(out, in[literalStart:literalStart+literalCount]...)
				literalStart += literalCount
			}

			// find match
			var i int
			inputPos += 3
			for i = 3; i < 9; i++ {
				inputPos++
				if in[matchPos+i] != in[inputPos-1] {
					break
				}
			}

			if i < 9 {
				inputPos--
				matchLen := inputPos - literalStart
				if matchOffset <= maxOffsetM2 {
					matchOffset--
					out = append(out,
						byte((((matchLen - 1) << 5) | ((matchOffset & 7) << 2))),
						byte((matchOffset >> 3)))
				} else if matchOffset <= maxOffsetM3 {
					matchOffset--
					out = append(out,
						byte(markerM3|(matchLen-2)),
						byte((matchOffset&63)<<2),
						byte(matchOffset>>6))
				} else {
					matchOffset -= 0x4000
					out = append(out,
						byte(markerM4|((matchOffset&0x4000)>>11)|(matchLen-2)),
						byte((matchOffset&63)<<2),
						byte(matchOffset>>6))
				}
			} else { // match len >= 9
				m := matchPos + maxLenM2 + 1
				for inputPos < inputLen && in[m] == in[inputPos] {
					m++
					inputPos++
				}
				matchLen := inputPos - literalStart
				if matchOffset <= maxOffsetM3 {
					matchOffset--
					if matchLen <= 33 {
						out = append(out, byte(markerM3|(matchLen-2)))
					} else {
						matchLen -= 33
						out = append(out, byte(markerM3))
						out = appendMultiple(out, matchLen)
					}
				} else {
					matchOffset -= 0x4000
					if matchLen <= maxLenM4 {
						out = append(out, byte(markerM4|((matchOffset&0x4000)>>11)|(matchLen-2)))
					} else {
						matchLen -= maxLenM4
						out = append(out, byte(markerM4|((matchOffset&0x4000)>>11)))
						out = appendMultiple(out, matchLen)
					}
				}
				out = append(out, byte((matchOffset&63)<<2), byte(matchOffset>>6))
			}

			literalStart = inputPos
			if inputPos >= inputLimit {
				break
			}

			continue
		}

		// literal
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
		out, literalTailSize = compress(in)
	}

	if literalTailSize > 0 {
		ii := inLen - literalTailSize
		if len(out) == 0 && literalTailSize <= 238 {
			out = append(out, byte(17+literalTailSize))
		} else if literalTailSize <= 3 {
			out[len(out)-2] |= byte(literalTailSize)
		} else if literalTailSize <= 18 {
			out = append(out, byte(literalTailSize-3))
		} else {
			out = append(out, 0)
			out = appendMultiple(out, literalTailSize-18)
		}
		out = append(out, in[ii:ii+literalTailSize]...)
	}

	out = append(out, markerM4|1, 0, 0)
	return out
}

// findCandidate returns (matchPos, matchOffset) for the given dict slot, or (-1, 0) if none.
func findCandidate(dict []int32, in []byte, inputPos, dictIndex int) (matchPos int, matchOffset int) {
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

// appendMultiple appends a multiple of 255 to the output.
func appendMultiple(out []byte, t int) []byte {
	for t > 255 {
		out = append(out, 0)
		t -= 255
	}

	out = append(out, byte(t))
	return out
}
