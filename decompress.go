package lzo

// Decompress decompresses LZO1X data from src into a buffer of length opts.OutLen.
// Buffer-based: single pre-allocated output, no append.
// Returns ErrOptionsRequired if opts is nil (OutLen is required).
func Decompress(src []byte, opts *DecompressOptions) ([]byte, error) {
	if opts == nil {
		return nil, ErrOptionsRequired
	}

	outLen := opts.OutLen
	if len(src) == 0 {
		return nil, ErrEmptyInput
	}

	dst := make([]byte, outLen)
	inputPos := 0
	outputPos := 0
	inputLen := len(src)

	copyMatch := func(distance, length int) error {
		if err := copyBackRef(dst, outputPos, distance, length); err != nil {
			return err
		}

		outputPos += length
		return nil
	}

	// First byte: LZO1X allows initial literal run if instruction > 17.
	instruction := int(src[inputPos])
	inputPos++
	if instruction > 17 {
		instruction -= 17
		if inputPos+instruction > inputLen || outputPos+instruction > outLen {
			return nil, ErrInputOverrun
		}

		copy(dst[outputPos:outputPos+instruction], src[inputPos:inputPos+instruction])
		inputPos += instruction
		outputPos += instruction
		if inputPos >= inputLen {
			return nil, ErrUnexpectedEOF
		}

		instruction = int(src[inputPos])
		inputPos++
	}

	// Main decode loop. instruction: <16 = short literal run + 3-byte match; >=64 = M2 match;
	// >=32 = M2 long; 16..31 = M3 match or stream terminator. Trailing literals (0–3 bytes) come from low 2 bits of the instruction or offset byte (trailingLitSource).
	for {
		var matchDist, matchLen int
		var trailingLitSource int

		if instruction < 16 {
			// Short literal run: instruction may be extended by zero bytes (instruction==0 → read 255s then final byte).
			if instruction == 0 {
				for {
					if inputPos >= inputLen {
						return nil, ErrInputOverrun
					}

					b := src[inputPos]
					inputPos++
					if b != 0 {
						instruction += 15 + int(b)
						break
					}

					instruction += 255
				}
			}

			if inputPos+instruction+3 > inputLen || outputPos+instruction+3 > outLen {
				return nil, ErrInputOverrun
			}

			copy(dst[outputPos:outputPos+instruction], src[inputPos:inputPos+instruction])
			outputPos += instruction
			inputPos += instruction

			// After literals: fixed 3-byte match; offset = (1+0x0800) + (instruction>>2) + (next_byte<<2).
			if inputPos >= inputLen {
				return nil, ErrInputOverrun
			}

			b := int(src[inputPos])
			inputPos++
			trailingLitSource = instruction
			matchDist = (1 + 0x0800) + (instruction >> 2) + (b << 2)
			matchLen = 3
			if err := copyMatch(matchDist, matchLen); err != nil {
				return nil, err
			}
		} else if instruction >= 64 {
			// M2 match: offset from (instruction>>2)&7 and next byte <<3, length from (instruction>>5)-1+2.
			if inputPos >= inputLen {
				return nil, ErrInputOverrun
			}

			b := int(src[inputPos])
			inputPos++
			matchDist = ((instruction >> 2) & 7) + (b << 3) + 1
			matchLen = (instruction >> 5) - 1 + 2
			trailingLitSource = instruction
			if err := copyMatch(matchDist, matchLen); err != nil {
				return nil, err
			}
		} else if instruction >= 32 {
			// M2 long: length in instruction&31 (or zero-extended), then 2-byte offset.
			matchLen = instruction & 31
			if matchLen == 0 {
				for {
					if inputPos >= inputLen {
						return nil, ErrInputOverrun
					}

					b := src[inputPos]
					inputPos++
					if b != 0 {
						matchLen += 31 + int(b)
						break
					}

					matchLen += 255
				}
			} else {
				matchLen += 31
			}

			if inputPos+2 > inputLen {
				return nil, ErrInputOverrun
			}

			b1 := int(src[inputPos])
			inputPos++
			b2 := int(src[inputPos])
			inputPos++
			matchDist = (b1 >> 2) + (b2 << 6) + 1
			matchLen += 2
			trailingLitSource = b1
			if err := copyMatch(matchDist, matchLen); err != nil {
				return nil, err
			}
		} else {
			// M3 (16 <= instruction < 32) or stream terminator (dist=0x4000, len=1).
			mLen := (instruction & 8) << 11
			matchLen = instruction & 7
			if matchLen == 0 {
				for {
					if inputPos >= inputLen {
						return nil, ErrInputOverrun
					}

					b := src[inputPos]
					inputPos++
					if b != 0 {
						matchLen += 7 + int(b)
						break
					}

					matchLen += 255
				}
			} else {
				matchLen += 7
			}

			if inputPos+2 > inputLen {
				return nil, ErrInputOverrun
			}

			b1 := int(src[inputPos])
			inputPos++
			b2 := int(src[inputPos])
			inputPos++
			matchDist = (b1 >> 2) + (b2 << 6) + mLen
			if matchDist == 0x4000 && matchLen == 1 {
				return dst[:outputPos], nil
			}

			if matchDist == 0 {
				return dst[:outputPos], nil
			}

			matchDist += 0x4000
			matchLen += 2
			trailingLitSource = b1
			if err := copyMatch(matchDist, matchLen); err != nil {
				return nil, err
			}
		}

		// Trailing literals: LZO1X encodes 0–3 bytes after each match in the low 2 bits
		// of the instruction byte (or the first offset byte for M2/M3); no separate opcode.
		litCount := trailingLitSource & 3
		if litCount > 0 {
			if inputPos+litCount > inputLen || outputPos+litCount > outLen {
				return nil, ErrInputOverrun
			}

			copy(dst[outputPos:outputPos+litCount], src[inputPos:inputPos+litCount])
			outputPos += litCount
			inputPos += litCount
		}

		if inputPos >= inputLen {
			return nil, ErrUnexpectedEOF
		}

		instruction = int(src[inputPos])
		inputPos++
	}
}
