// SPDX-License-Identifier: GPL-2.0-only
// Source: github.com/woozymasta/lzo

package lzo

// initMatcher initializes match finding state.
func (c *lzoCompressor) initMatcher(dict *slidingWindowDict, flags uint32) error {
	dict.compressor = c
	if err := dict.init(); err != nil {
		return err
	}

	if flags&1 != 0 {
		dict.UseBestOff = true
	}

	return nil
}

// advanceMatchFinder updates dictionary state and finds the best next match.
func (c *lzoCompressor) advanceMatchFinder(dict *slidingWindowDict, thislen uint, skip uint) error {
	if skip > 0 {
		if thislen < skip {
			return ErrCompressInternal
		}

		if err := dict.accept(thislen - skip); err != nil {
			return err
		}

		c.textsize += thislen - skip + 1
	} else {
		if thislen > 1 {
			return ErrCompressInternal
		}

		c.textsize += thislen - skip
	}

	dict.MLen = swdMinMatchLen
	dict.MOff = 0
	for i := range dict.bestPos {
		dict.bestPos[i] = 0
	}

	if err := dict.findBestMatch(); err != nil {
		return err
	}

	c.matchLen = int(dict.MLen)    //nolint:gosec // G115: MLen bounded by window
	c.matchOffset = int(dict.MOff) //nolint:gosec // G115: MOff bounded by window

	dict.getByte()
	if dict.BChar < 0 {
		c.lookAhead = 0
		c.matchLen = 0
	} else {
		c.lookAhead = dict.Look + 1
	}

	c.bufPos = c.inputPos - int(c.lookAhead) //nolint:gosec // G115: lookAhead bounded

	return nil
}

// adjustMatchForOffsetClass tries to shorten the match (matchLen, matchOffset) so it fits a smaller
// offset class (M2/M3), using the sliding window's best-offset table. Returns the
// possibly adjusted match length and offset.
func (c *lzoCompressor) adjustMatchForOffsetClass(dict *slidingWindowDict, inputMatchLen, inputMatchOffset int) (matchLen int, matchOffset int) {
	matchLen = inputMatchLen
	matchOffset = inputMatchOffset
	if matchLen <= minLenM2 {
		return
	}
	if matchOffset <= maxOffsetM2 {
		return
	}

	if matchOffset > maxOffsetM2 && matchLen >= minLenM2+1 && matchLen <= maxLenM2+1 &&
		dict.BestOff[matchLen-1] > 0 && dict.BestOff[matchLen-1] <= maxOffsetM2 {
		matchLen--
		matchOffset = int(dict.BestOff[matchLen]) //nolint:gosec // G115: BestOff values bounded
		return
	}

	if matchOffset > maxOffsetM3 && matchLen >= maxLenM4+1 && matchLen <= maxLenM2+2 &&
		dict.BestOff[matchLen-2] > 0 && dict.BestOff[matchLen-2] <= maxOffsetM2 {
		matchLen -= 2
		matchOffset = int(dict.BestOff[matchLen]) //nolint:gosec // G115: BestOff values bounded
		return
	}

	if matchOffset > maxOffsetM3 && matchLen >= maxLenM4+1 && matchLen <= maxLenM3+1 &&
		dict.BestOff[matchLen-1] > 0 && dict.BestOff[matchLen-1] <= maxOffsetM3 {
		matchLen--
		matchOffset = int(dict.BestOff[matchLen]) //nolint:gosec // G115: BestOff values bounded
		return
	}

	return
}
