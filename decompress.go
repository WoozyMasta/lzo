package lzo

import "io"

// shortMatchBaseOffset is the base for the 3-byte match distance after a short literal run (0x0800).
const shortMatchBaseOffset = 0x0800

// decodeState is the current state of the LZO1X decoder state machine.
type decodeState int

const (
	stateBeginLoop decodeState = iota
	stateFirstLiteralRun
	stateMatch
	stateMatchDone
	stateMatchNext
	stateMatchEnd
)

// Decompress decompresses LZO1X data from src into a buffer of length opts.OutLen.
// Returns ErrOptionsRequired if opts is nil; ErrEmptyInput if src is empty.
// On success returns the decompressed slice (length may be less than OutLen if stream ended with terminator).
func Decompress(src []byte, opts *DecompressOptions) ([]byte, error) {
	if opts == nil {
		return nil, ErrOptionsRequired
	}

	if len(src) == 0 {
		return nil, ErrEmptyInput
	}

	outLen := opts.OutLen
	if outLen < 0 {
		return nil, ErrOptionsRequired
	}

	dst := make([]byte, outLen)
	n, _, err := decompressCore(src, dst)
	if err != nil {
		return nil, err
	}

	return dst[:n], nil
}

// DecompressN decompresses LZO1X data from src and returns the decoded slice,
// the number of input bytes consumed (nRead), and an error.
// nRead is 0 on error. Use this when advancing a stream (e.g. back-to-back compressed blocks).
func DecompressN(src []byte, opts *DecompressOptions) ([]byte, int, error) {
	if opts == nil {
		return nil, 0, ErrOptionsRequired
	}

	if len(src) == 0 {
		return nil, 0, ErrEmptyInput
	}

	outLen := opts.OutLen
	if outLen < 0 {
		return nil, 0, ErrOptionsRequired
	}

	dst := make([]byte, outLen)
	outWritten, inConsumed, err := decompressCore(src, dst)
	if err != nil {
		return nil, 0, err
	}

	return dst[:outWritten], inConsumed, nil
}

// DecompressFromReader reads the full stream then calls Decompress. No decoding logic of its own.
// If opts.MaxInputSize > 0 and more bytes are read, returns ErrInputTooLarge.
func DecompressFromReader(r io.Reader, opts *DecompressOptions) ([]byte, error) {
	if opts == nil {
		return nil, ErrOptionsRequired
	}

	src, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	if opts.MaxInputSize > 0 && len(src) > opts.MaxInputSize {
		return nil, ErrInputTooLarge
	}

	return Decompress(src, opts)
}

// decompressCore decompresses LZO1X data from src into dst using a state machine.
// It writes starting at dst[0] and returns (bytes written, input bytes consumed, nil) on success.
// On stream terminator it returns (outputOffset, inputOffset, nil). On error it returns (0, 0, err).
func decompressCore(src, dst []byte) (outWritten, inConsumed int, err error) {
	inputLen := len(src)
	outputLen := len(dst)
	if inputLen == 0 {
		return 0, 0, ErrEmptyInput
	}

	var (
		state              decodeState
		instructionByte    int
		trailingSourceByte byte
		runLength          int
		matchLength        int
		backRefDistance    int
		inputOffset        int
		outputOffset       int
	)

	// First byte: initial literal run or enter main loop.
	instructionByte = int(src[inputOffset])
	inputOffset++
	if instructionByte > 17 {
		runLength = instructionByte - 17
		if inputOffset+runLength > inputLen || outputOffset+runLength > outputLen {
			return 0, 0, ErrInputOverrun
		}

		copy(dst[outputOffset:outputOffset+runLength], src[inputOffset:inputOffset+runLength])
		inputOffset += runLength
		outputOffset += runLength
		if inputOffset >= inputLen {
			return 0, 0, ErrUnexpectedEOF
		}

		instructionByte = int(src[inputOffset])
		inputOffset++
		state = stateFirstLiteralRun
	} else {
		state = stateBeginLoop
	}

	for {
		switch state {
		case stateBeginLoop:
			if instructionByte >= 16 {
				state = stateMatch
				continue
			}

			runLength = instructionByte
			if runLength == 0 {
				var err error
				runLength, err = readZeroExtendedLength(src, &inputOffset, inputLen, 15)
				if err != nil {
					return 0, 0, err
				}
			}

			runLength += 3
			if inputOffset+runLength > inputLen || outputOffset+runLength > outputLen {
				return 0, 0, ErrInputOverrun
			}

			copy(dst[outputOffset:outputOffset+runLength], src[inputOffset:inputOffset+runLength])
			inputOffset += runLength
			outputOffset += runLength
			if inputOffset >= inputLen {
				return 0, 0, ErrInputOverrun
			}

			instructionByte = int(src[inputOffset])
			inputOffset++
			trailingSourceByte = byte(instructionByte)
			state = stateFirstLiteralRun

		case stateFirstLiteralRun:
			if instructionByte >= 16 {
				state = stateMatch
				continue
			}

			b, err := readByte(src, &inputOffset, inputLen)
			if err != nil {
				return 0, 0, err
			}

			backRefDistance = (1 + shortMatchBaseOffset) + (instructionByte >> 2) + (int(b) << 2)
			if err := copyBackRef(dst, outputOffset, backRefDistance, 3); err != nil {
				return 0, 0, err
			}

			outputOffset += 3
			state = stateMatchDone

		case stateMatch:
			trailingSourceByte = byte(instructionByte)
			switch {
			case instructionByte < 16:
				// 2-byte short match (M1-style): distance = 1 + (t>>2) + (next<<2), length 2
				b, err := readByte(src, &inputOffset, inputLen)
				if err != nil {
					return 0, 0, err
				}
				backRefDistance = 1 + (instructionByte >> 2) + (int(b) << 2)
				if err := copyBackRef(dst, outputOffset, backRefDistance, 2); err != nil {
					return 0, 0, err
				}
				outputOffset += 2
				state = stateMatchDone

			case instructionByte >= 64:
				// M2
				b, err := readByte(src, &inputOffset, inputLen)
				if err != nil {
					return 0, 0, err
				}

				backRefDistance = ((instructionByte >> 2) & 7) + (int(b) << 3) + 1
				matchLength = (instructionByte >> 5) - 1 + 2
				if err := copyBackRef(dst, outputOffset, backRefDistance, matchLength); err != nil {
					return 0, 0, err
				}

				outputOffset += matchLength
				state = stateMatchDone

			case instructionByte >= 32:
				// M2 long: length = (t&31)+2 or extended+2
				matchLength = instructionByte & 31
				if matchLength == 0 {
					var err error
					matchLength, err = readZeroExtendedLength(src, &inputOffset, inputLen, 31)
					if err != nil {
						return 0, 0, err
					}
				}
				matchLength += 2

				v16, err := readUint16LittleEndian(src, &inputOffset, inputLen)
				if err != nil {
					return 0, 0, err
				}

				backRefDistance = (int(v16) >> 2) + 1
				trailingSourceByte = byte(v16 & 0xFF)
				if err := copyBackRef(dst, outputOffset, backRefDistance, matchLength); err != nil {
					return 0, 0, err
				}

				outputOffset += matchLength
				state = stateMatchDone

			default:
				// M3 (16 <= instructionByte < 32) or terminator: length = (t&7)+2 or extended+2
				mLenHigh := (instructionByte & 8) << 11
				matchLength = instructionByte & 7
				if matchLength == 0 {
					var err error
					matchLength, err = readZeroExtendedLength(src, &inputOffset, inputLen, 7)
					if err != nil {
						return 0, 0, err
					}
				}
				matchLength += 2

				v16, err := readUint16LittleEndian(src, &inputOffset, inputLen)
				if err != nil {
					return 0, 0, err
				}

				backRefDistance = (int(v16) >> 2) + mLenHigh
				trailingSourceByte = byte(v16 & 0xFF)
				if backRefDistance == 0 {
					return outputOffset, inputOffset, nil
				}

				backRefDistance += 0x4000
				if err := copyBackRef(dst, outputOffset, backRefDistance, matchLength); err != nil {
					return 0, 0, err
				}

				outputOffset += matchLength
				state = stateMatchDone
			}

		case stateMatchDone:
			trailingCount := int(trailingSourceByte & 3)
			if trailingCount == 0 {
				state = stateMatchEnd
				continue
			}

			if inputOffset+trailingCount > inputLen || outputOffset+trailingCount > outputLen {
				return 0, 0, ErrInputOverrun
			}
			copy(dst[outputOffset:outputOffset+trailingCount], src[inputOffset:inputOffset+trailingCount])
			outputOffset += trailingCount
			inputOffset += trailingCount
			if inputOffset >= inputLen {
				return 0, 0, ErrUnexpectedEOF
			}

			instructionByte = int(src[inputOffset])
			inputOffset++
			state = stateMatchNext

		case stateMatchNext:
			state = stateMatch
			continue

		case stateMatchEnd:
			if inputOffset >= inputLen {
				return 0, 0, ErrUnexpectedEOF
			}
			instructionByte = int(src[inputOffset])
			inputOffset++
			state = stateBeginLoop
		}
	}
}

// readByte reads one byte from src at *inputOffset; advances *inputOffset. Returns ErrInputOverrun if at end.
func readByte(src []byte, inputOffset *int, inputLen int) (byte, error) {
	if *inputOffset >= inputLen {
		return 0, ErrInputOverrun
	}

	b := src[*inputOffset]
	*inputOffset++

	return b, nil
}

// readUint16LittleEndian reads two bytes little-endian from src at *inputOffset; advances *inputOffset by 2.
func readUint16LittleEndian(src []byte, inputOffset *int, inputLen int) (uint16, error) {
	if *inputOffset+2 > inputLen {
		return 0, ErrInputOverrun
	}

	lo := uint16(src[*inputOffset])
	hi := uint16(src[*inputOffset+1])
	*inputOffset += 2

	return lo | hi<<8, nil
}

// readZeroExtendedLength reads a length encoded as zero-or-more 0 bytes then base+final byte.
// Each 0 adds 255 to the length; the final non-zero byte b adds base+b.
func readZeroExtendedLength(src []byte, inputOffset *int, inputLen int, base int) (int, error) {
	length := 0
	for {
		if *inputOffset >= inputLen {
			return 0, ErrInputOverrun
		}

		b := src[*inputOffset]
		*inputOffset++
		if b != 0 {
			length += base + int(b)
			return length, nil
		}

		length += 255
	}
}
