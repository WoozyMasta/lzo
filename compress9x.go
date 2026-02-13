// SPDX-License-Identifier: GPL-2.0-only
// Source: github.com/woozymasta/lzo

package lzo

// lzoCompressor holds state for LZO1X-999 compression of a single input buffer.
type lzoCompressor struct {
	// Input and positions

	input    []byte // source data
	inputPos int    // current read position in input
	bufPos   int    // current position in compressor's logical buffer (inputPos - lookahead)

	// Statistics (counts of emitted bytes)

	matchBytes int // number of bytes encoded as matches
	litBytes   int // number of bytes encoded as literals
	lazy       int // number of lazy matches encoded

	// Encoding state: previous run's literal count and match length (for M1 encoding)

	lastRunLiteralCount int  // number of bytes encoded as literals in the previous run
	lastRunMatchLen     int  // length of the previous match encoded
	m1am                uint // number of matches encoded as M1
	m2m                 uint // number of matches encoded as M2
	m1bm                uint // number of matches encoded as M1 with a previous run of literals
	m3m                 uint // number of matches encoded as M3
	m4m                 uint // number of matches encoded as M4
	lit1r               uint // number of runs of literals encoded as 1 byte
	lit2r               uint // number of runs of literals encoded as 2 bytes
	lit3r               uint // number of runs of literals encoded as 3 bytes

	// Last encoded match (for stats / optional use)

	lastEncodedMatchLen    int  // length of the last encoded match
	lastEncodedMatchOffset int  // offset of the last encoded match
	textsize               uint // total number of bytes encoded

	// Current match from sliding-window search

	matchLen    int  // length of best match
	matchOffset int  // backward distance of best match
	lookAhead   uint // number of bytes available in lookahead
}

// Compress1X999Level compresses in with LZO1X-999 at the given level (1–9).
// Higher levels give better compression and are slower.
func Compress1X999Level(in []byte, level int) ([]byte, error) {
	return compress9x(in, fixedLevels[level-1])
}

// Compress1X999 compresses in with LZO1X-999 at level 9 (best ratio).
func Compress1X999(in []byte) ([]byte, error) {
	return Compress1X999Level(in, 9)
}

// compress9x implements LZO1X-999 for the given level parameters (called by Compress1X999Level).
func compress9x(in []byte, p compressLevelParams) ([]byte, error) {
	ctx := lzoCompressor{}
	swDict := acquireSlidingWindowDict()
	defer releaseSlidingWindowDict(swDict)

	if p.tryLazy < 0 {
		p.tryLazy = 1
	}
	if p.goodLen == 0 {
		p.goodLen = 32
	}
	if p.maxLazy == 0 {
		p.maxLazy = 32
	}
	if p.maxChain == 0 {
		p.maxChain = swdDefaultMaxChain
	}

	ctx.input = in

	out := make([]byte, 0, len(in)/2)
	literalStart := 0
	literalCount := 0

	if err := ctx.initMatcher(swDict, p.flags); err != nil {
		return nil, err
	}

	if p.maxChain > 0 {
		swDict.MaxChain = p.maxChain
	}
	if p.niceLen > 0 {
		swDict.NiceLength = p.niceLen
	}

	if err := ctx.advanceMatchFinder(swDict, 0, 0); err != nil {
		return nil, err
	}

	for ctx.lookAhead > 0 {
		currentMatchLen := ctx.matchLen
		currentMatchOffset := ctx.matchOffset

		if literalCount == 0 {
			literalStart = ctx.bufPos
		}

		if currentMatchLen < 2 ||
			(currentMatchLen == 2 && (currentMatchOffset > maxOffsetM1 || literalCount == 0 || literalCount >= 4)) ||
			(currentMatchLen == 2 && len(out) == 0) ||
			(len(out) == 0 && literalCount == 0) {
			currentMatchLen = 0
		} else if currentMatchLen == minLenM2 {
			if currentMatchOffset > maxOffsetMX && literalCount >= 4 {
				currentMatchLen = 0
			}
		}

		if currentMatchLen == 0 {
			literalCount++
			swDict.MaxChain = p.maxChain
			if err := ctx.advanceMatchFinder(swDict, 1, 0); err != nil {
				return nil, err
			}

			continue
		}

		if swDict.UseBestOff {
			currentMatchLen, currentMatchOffset = ctx.adjustMatchForOffsetClass(swDict, currentMatchLen, currentMatchOffset)
		}

		ahead := 0
		encodedLenCurrentMatch := 0
		maxahead := 0
		if p.tryLazy != 0 && currentMatchLen < int(p.maxLazy) { //nolint:gosec // G115: maxLazy is small
			var ok bool
			encodedLenCurrentMatch, ok = ctx.lenOfCodedMatch(currentMatchLen, currentMatchOffset, literalCount)
			if !ok {
				return nil, ErrCompressInternal
			}

			maxahead = min(p.tryLazy, encodedLenCurrentMatch-1)
		}

		matchDone := false
		for ahead < maxahead && int(ctx.lookAhead) > currentMatchLen {
			if currentMatchLen >= int(p.goodLen) { //nolint:gosec // G115: goodLen bounded
				swDict.MaxChain = p.maxChain >> 2
			} else {
				swDict.MaxChain = p.maxChain
			}

			if err := ctx.advanceMatchFinder(swDict, 1, 0); err != nil {
				return nil, err
			}

			ahead++

			if ctx.matchLen < currentMatchLen {
				continue
			}
			if ctx.matchLen == currentMatchLen && ctx.matchOffset >= currentMatchOffset {
				continue
			}

			if swDict.UseBestOff {
				ctx.matchLen, ctx.matchOffset = ctx.adjustMatchForOffsetClass(swDict, ctx.matchLen, ctx.matchOffset)
			}
			encodedLenBestMatch, ok := ctx.lenOfCodedMatch(ctx.matchLen, ctx.matchOffset, literalCount+ahead)
			if !ok {
				continue
			}

			encodedLenCompensation := 0
			if len(out) > 0 {
				encodedLenCompensation, _ = ctx.lenOfCodedMatch(ahead, currentMatchOffset, literalCount)
			}
			minLazyGain := ctx.minLazyMatchGain(
				ahead,
				literalCount,
				literalCount+ahead,
				encodedLenCurrentMatch,
				encodedLenBestMatch,
				encodedLenCompensation,
			)

			if ctx.matchLen >= currentMatchLen+minLazyGain {
				ctx.lazy++

				var err error
				if encodedLenCompensation > 0 {
					out, err = ctx.codeRun(out, literalStart, literalCount, ahead)
					if err != nil {
						return nil, err
					}
					literalCount = 0
					out, err = ctx.codeMatch(out, ahead, currentMatchOffset)
					if err != nil {
						return nil, err
					}
				} else {
					literalCount += ahead
				}
				matchDone = true
				break
			}
		}

		// If no match was found, encode the current match.
		if !matchDone {
			var err error
			out, err = ctx.codeRun(out, literalStart, literalCount, currentMatchLen)
			if err != nil {
				return nil, err
			}

			literalCount = 0
			out, err = ctx.codeMatch(out, currentMatchLen, currentMatchOffset)
			if err != nil {
				return nil, err
			}

			swDict.MaxChain = p.maxChain
			if err := ctx.advanceMatchFinder(swDict, uint(currentMatchLen), uint(1+ahead)); err != nil { //nolint:gosec // G115: match len and skip bounded
				return nil, err
			}
		}
	}

	if literalCount > 0 {
		out = ctx.storeRun(out, literalStart, literalCount)
	}

	out = append(out, markerM4|1, 0, 0)
	return out, nil
}

// codeMatch appends the LZO encoding for a match of length matchLen at backward distance matchOffset.
func (ctx *lzoCompressor) codeMatch(out []byte, matchLen int, matchOffset int) ([]byte, error) {
	savedMatchLen, savedMatchOffset := matchLen, matchOffset
	ctx.matchBytes += matchLen

	switch {
	case matchLen == 2: // match length 2, offset within 1-1023 bytes
		if matchOffset > maxOffsetM1 {
			return nil, ErrCompressInternal
		}
		if ctx.lastRunLiteralCount < 1 || ctx.lastRunLiteralCount >= 4 {
			return nil, ErrCompressInternal
		}
		matchOffset--
		out = append(out,
			markerM1|byte((matchOffset&3)<<2),
			byte(matchOffset>>2))
		ctx.m1am++

	case matchLen <= maxLenM2 && matchOffset <= maxOffsetM2: // match length 3–8, offset within 1-2047
		if matchLen < 3 {
			return nil, ErrCompressInternal
		}
		matchOffset--
		out = append(out,
			byte((matchLen-1)<<5|(matchOffset&7)<<2),
			byte(matchOffset>>3))
		if out[len(out)-2] < markerM2 {
			return nil, ErrCompressInternal
		}
		ctx.m2m++

	case matchLen == minLenM2 && matchOffset <= maxOffsetMX && ctx.lastRunLiteralCount >= 4:
		if matchLen != 3 {
			return nil, ErrCompressInternal
		}
		if matchOffset <= maxOffsetM2 {
			return nil, ErrCompressInternal
		}
		matchOffset -= 1 + maxOffsetM2
		out = append(out,
			byte(markerM1|((matchOffset&3)<<2)),
			byte(matchOffset>>2))
		ctx.m1bm++

	case matchOffset <= maxOffsetM3: // offset within 1-8191
		if matchLen < 3 {
			return nil, ErrCompressInternal
		}
		matchOffset--
		if matchLen <= maxLenM3 {
			out = append(out, byte(markerM3|(matchLen-2)))
		} else {
			matchLen -= maxLenM3
			out = append(out, byte(markerM3))
			out = appendMultiple(out, matchLen)
		}
		out = append(out, byte(matchOffset<<2), byte(matchOffset>>6))
		ctx.m3m++

	default: // offset 8192-16383
		if matchLen < 3 {
			return nil, ErrCompressInternal
		}
		if matchOffset <= 0x4000 || matchOffset >= 0xc000 {
			return nil, ErrCompressInternal
		}
		matchOffset -= 0x4000
		k := (matchOffset & 0x4000) >> 11
		if matchLen <= maxLenM4 {
			out = append(out, byte(markerM4|k|(matchLen-2)))
		} else {
			matchLen -= maxLenM4
			out = append(out, byte(markerM4|k))
			out = appendMultiple(out, matchLen)
		}
		out = append(out, byte(matchOffset<<2), byte(matchOffset>>6))
		ctx.m4m++
	}

	ctx.lastEncodedMatchLen = savedMatchLen
	ctx.lastEncodedMatchOffset = savedMatchOffset
	return out, nil
}

// storeRun appends the encoding for a literal run of length literalCount starting at literalStart.
func (ctx *lzoCompressor) storeRun(out []byte, literalStart int, literalCount int) []byte {
	ctx.litBytes += literalCount

	switch {
	case len(out) == 0 && literalCount <= 238:
		out = append(out, byte(17+literalCount))
	case literalCount <= 3:
		out[len(out)-2] |= byte(literalCount)
		ctx.lit1r++
	case literalCount <= 18:
		out = append(out, byte(literalCount-3))
		ctx.lit2r++
	default:
		out = append(out, 0)
		out = appendMultiple(out, literalCount-18)
		ctx.lit3r++
	}

	out = append(out, ctx.input[literalStart:literalStart+literalCount]...)
	return out
}

// codeRun encodes a run: optional literals (literalStart, literalCount) then match of matchLen.
// Updates lastRunLiteralCount and lastRunMatchLen for the next potential M1 short match.
func (ctx *lzoCompressor) codeRun(out []byte, literalStart int, literalCount int, matchLen int) ([]byte, error) {
	if literalCount > 0 {
		if matchLen < 2 {
			return nil, ErrCompressInternal
		}

		out = ctx.storeRun(out, literalStart, literalCount)
		ctx.lastRunMatchLen = matchLen
		ctx.lastRunLiteralCount = literalCount
	} else {
		if matchLen < 3 {
			return nil, ErrCompressInternal
		}

		ctx.lastRunMatchLen = 0
		ctx.lastRunLiteralCount = 0
	}

	return out, nil
}

// lenOfCodedMatch returns the number of bytes needed to encode a match (matchLen, matchOffset)
// with the given preceding literalCount. Second return is false if the match is not encodable.
func (ctx *lzoCompressor) lenOfCodedMatch(matchLen int, matchOffset int, literalCount int) (int, bool) {
	switch {
	case matchLen < 2:
		return 0, false

	case matchLen == 2:
		if matchOffset <= maxOffsetM1 && literalCount > 0 && literalCount < 4 {
			return 2, true
		}
		return 0, false

	case matchLen <= maxLenM2 && matchOffset <= maxOffsetM2:
		return 2, true

	case matchLen == minLenM2 && matchOffset <= maxOffsetMX && literalCount >= 4:
		return 2, true

	case matchOffset <= maxOffsetM3:
		if matchLen <= maxLenM3 {
			return 3, true
		}

		n := 4
		matchLen -= maxLenM3
		for matchLen > 255 {
			matchLen -= 255
			n++
		}
		return n, true

	case matchOffset <= maxOffsetM4:
		if matchLen <= maxLenM4 {
			return 3, true
		}

		n := 4
		matchLen -= maxLenM4
		for matchLen > 255 {
			matchLen -= 255
			n++
		}
		return n, true

	default:
		return 0, false
	}
}

func (ctx *lzoCompressor) minLazyMatchGain(ahead int, literalCountCurrent int, literalCountLazy int, currentMatchEncodedLen int, bestMatchEncodedLen int, compensationEncodedLen int) int {
	if ahead <= 0 {
		return 0
	}

	minimumGain := ahead
	if literalCountCurrent <= 3 {
		if literalCountLazy > 3 {
			minimumGain += 2
		}
	} else if literalCountCurrent <= 18 {
		if literalCountLazy > 18 {
			minimumGain++
		}
	}

	minimumGain += (bestMatchEncodedLen - currentMatchEncodedLen) * 2
	if compensationEncodedLen != 0 {
		minimumGain -= (ahead - compensationEncodedLen) * 2
	}
	if minimumGain < 0 {
		minimumGain = 0
	}

	return minimumGain
}
