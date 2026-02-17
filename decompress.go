// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/lzo

package lzo

import "io"

const (
	// shortMatchBaseOffset is the base distance used by the short-match form
	// selected when the parser is in state 4.
	shortMatchBaseOffset = 0x0800

	// maxZeroExtendedChunks limits zero-extension runs so malformed inputs cannot
	// overflow run-length reconstruction math.
	maxZeroExtendedChunks = int(^uint(0)/255) - 2
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
	if len(src) == 0 {
		return 0, 0, ErrEmptyInput
	}

	var (
		inst      byte
		state     int
		nextState int
		matchLen  int
		matchDist int
		inPos     int
		outPos    int
	)

	inst, err = readCompressedByte(src, &inPos)
	if err != nil {
		return 0, 0, err
	}

	// First byte can encode an initial literal run directly; otherwise it becomes
	// the first instruction in the main decode loop.
	switch {
	case inst >= 22:
		if err := copyLiteralRun(src, &inPos, dst, &outPos, int(inst)-17); err != nil {
			return 0, 0, err
		}
		state = 4

	case inst >= 18:
		nextState = int(inst - 17)
		if err := copyLiteralRun(src, &inPos, dst, &outPos, nextState); err != nil {
			return 0, 0, err
		}
		state = nextState
	}

	for {
		// `inst` is already loaded for the very first iteration.
		if inPos > 1 || state > 0 {
			if inPos >= len(src) {
				return 0, 0, ErrUnexpectedEOF
			}

			inst = src[inPos]
			inPos++
		}

		switch {
		case inst >= markerM2:
			b, err := readCompressedByte(src, &inPos)
			if err != nil {
				return 0, 0, err
			}

			matchDist = (int(b) << 3) + ((int(inst) >> 2) & 0x7) + 1
			matchLen = (int(inst) >> 5) + 1
			nextState = int(inst & 0x03)

		case inst >= markerM3:
			matchLen = int(inst&0x1f) + 2
			if matchLen == 2 {
				ext, err := readZeroExtendedChunks(src, &inPos)
				if err != nil {
					return 0, 0, err
				}

				tail, err := readCompressedByte(src, &inPos)
				if err != nil {
					return 0, 0, err
				}

				matchLen += ext*255 + 31 + int(tail)
			}

			v16, err := readCompressedLE16(src, &inPos)
			if err != nil {
				return 0, 0, err
			}

			matchDist = (int(v16) >> 2) + 1
			nextState = int(v16 & 0x03)

		case inst >= markerM4:
			matchLen = int(inst&0x7) + 2
			if matchLen == 2 {
				ext, err := readZeroExtendedChunks(src, &inPos)
				if err != nil {
					return 0, 0, err
				}

				tail, err := readCompressedByte(src, &inPos)
				if err != nil {
					return 0, 0, err
				}

				matchLen += ext*255 + 7 + int(tail)
			}

			v16, err := readCompressedLE16(src, &inPos)
			if err != nil {
				return 0, 0, err
			}

			baseDist := ((int(inst) & 0x8) << 11) + (int(v16) >> 2)
			if baseDist == 0 {
				// Stream terminator is encoded as M4 with distance=0 and length=3.
				if matchLen != 3 {
					return 0, 0, ErrInputOverrun
				}

				return outPos, inPos, nil
			}

			matchDist = baseDist + 0x4000
			nextState = int(v16 & 0x03)

		default:
			if state == 0 {
				// In state 0, this opcode form encodes a literal-run length directly
				// (with optional zero-extension for long runs).
				runLen := int(inst) + 3
				if runLen == 3 {
					ext, err := readZeroExtendedChunks(src, &inPos)
					if err != nil {
						return 0, 0, err
					}

					tail, err := readCompressedByte(src, &inPos)
					if err != nil {
						return 0, 0, err
					}

					runLen += ext*255 + 15 + int(tail)
				}

				if err := copyLiteralRun(src, &inPos, dst, &outPos, runLen); err != nil {
					return 0, 0, err
				}

				// Keep historical behavior: a plain literal-run stream without terminator is malformed.
				if inPos >= len(src) {
					return 0, 0, ErrInputOverrun
				}

				state = 4
				continue
			}

			// In non-zero states this opcode form is a short back-reference and
			// needs one trailing byte to complete distance bits.
			tail, err := readCompressedByte(src, &inPos)
			if err != nil {
				return 0, 0, err
			}

			nextState = int(inst & 0x03)
			switch {
			case state != 4:
				// General short-match form: fixed length 2, distance starts at 1.
				matchDist = (int(inst) >> 2) + (int(tail) << 2) + 1
				matchLen = 2

			default:
				// Special short-match form used after a 4-literal tail.
				matchDist = shortMatchBaseOffset + 1 + (int(inst) >> 2) + (int(tail) << 2)
				matchLen = 3
			}
		}

		if err := copyBackRef(dst, outPos, matchDist, matchLen); err != nil {
			return 0, 0, err
		}

		outPos += matchLen
		if nextState > 0 {
			if err := copyLiteralRun(src, &inPos, dst, &outPos, nextState); err != nil {
				return 0, 0, err
			}
		}

		state = nextState
	}
}

// readCompressedByte reads one byte from src at *inPos and advances *inPos.
func readCompressedByte(src []byte, inPos *int) (byte, error) {
	if *inPos >= len(src) {
		return 0, ErrInputOverrun
	}

	b := src[*inPos]
	*inPos++

	return b, nil
}

// readCompressedLE16 reads one little-endian uint16 from src at *inPos and advances *inPos by 2.
func readCompressedLE16(src []byte, inPos *int) (uint16, error) {
	if *inPos+2 > len(src) {
		return 0, ErrInputOverrun
	}

	lo := uint16(src[*inPos])
	hi := uint16(src[*inPos+1])
	*inPos += 2

	return lo | hi<<8, nil
}

// readZeroExtendedChunks consumes consecutive zero bytes and returns their count.
func readZeroExtendedChunks(src []byte, inPos *int) (int, error) {
	start := *inPos
	for *inPos < len(src) && src[*inPos] == 0 {
		*inPos++
	}

	count := *inPos - start
	if count > maxZeroExtendedChunks {
		return 0, ErrInputOverrun
	}

	return count, nil
}

// copyLiteralRun copies `n` bytes from src[*inPos:] to dst[*outPos:] and advances both pointers.
func copyLiteralRun(src []byte, inPos *int, dst []byte, outPos *int, n int) error {
	if n == 0 {
		return nil
	}

	if *inPos+n > len(src) {
		return ErrInputOverrun
	}

	if *outPos+n > len(dst) {
		return ErrOutputOverrun
	}

	copy(dst[*outPos:*outPos+n], src[*inPos:*inPos+n])
	*inPos += n
	*outPos += n

	return nil
}
