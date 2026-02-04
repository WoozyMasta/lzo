package lzo

// slidingWindowDict is the sliding-window dictionary for LZO1X-999 match finding
// (in LZO sources often abbreviated as SWD). It holds the ring buffer and hash chains.

const (
	swdWindowSize      = maxOffsetM4  // ring buffer size
	swdMinMatchLen     = 1            // lower limit for match length
	swdMaxLookahead    = 2048         // upper limit for match length
	swdBestOffCount    = maxLenM3 + 1 // max(m2,m3,m4)+1
	swdHashSize        = 16384
	swdDefaultMaxChain = 2048
)

// slidingWindowDict fields: window/config, match output, internal positions and hash chains.
type slidingWindowDict struct {
	compressor *lzoCompressor // Compressor reference

	bufferWrap []byte                // wrap region for lookahead
	BestOff    [swdBestOffCount]uint // best offset by match length (for M2/M3 encoding)
	bestPos    [swdBestOffCount]uint // best position by match length (for M2/M3 encoding)

	// Window size and limits (from constants)

	SwdN         uint // window size
	SwdF         uint // lookahead size
	SwdThreshold uint // minimum match length

	// Configuration

	MaxChain   uint // maximum chain length
	NiceLength uint // nice length
	LazyInsert uint // lazy insert

	// Match output

	MLen     uint // match length
	MOff     uint // match offset
	Look     uint // lookahead size
	BChar    int  // first byte of the best match
	matchPos uint // match position

	// Ring buffer positions and state

	insertPos      uint // next write position in buffer
	scanPos        uint // current scan (lookahead) position
	removePos      uint // position of node to remove from hash
	bufferSize     uint // size of the ring buffer
	nodeCount      uint // number of nodes in the hash chains
	firstRemovePos uint // position of the first node to remove from hash

	// Hash chains: 2-byte and 3-byte

	hashHead2    [65536]uint16                           // hash head for 2-byte keys (0 means empty, stored as pos+1)
	chainNext    [swdWindowSize + swdMaxLookahead]uint16 // next node in the hash chain
	chainBestLen [swdWindowSize + swdMaxLookahead]uint16 // best length of the hash chain
	hashHead3    [swdHashSize]uint16                     // hash head for 3-byte keys
	hashChainLen [swdHashSize]uint16                     // length of the hash chains

	// Ring buffer and wrap region for lookahead
	buffer [swdWindowSize + swdMaxLookahead + swdMaxLookahead]byte

	// use best offset by match length (for M2/M3 encoding)
	UseBestOff bool
}

// head2 returns the 2-byte hash key for the given data.
func head2(data []byte) uint {
	return uint(data[1])<<8 | uint(data[0])
}

// head3 returns the 3-byte hash key for the given data.
func head3(data []byte) uint {
	key := uint(data[0])
	key = (key << 5) ^ uint(data[1])
	key = (key << 5) ^ uint(data[2])
	key = (key * 0x9f5f) >> 5
	return key & (swdHashSize - 1)
}

// headForKey3 returns the chain head for the given 3-byte hash key.
func (s *slidingWindowDict) headForKey3(key uint) uint16 {
	if s.hashChainLen[key] == 0 {
		return 0xFFFF
	}

	return s.hashHead3[key]
}

// init initializes the sliding window dictionary.
func (s *slidingWindowDict) init() error {
	s.SwdN = swdWindowSize
	s.SwdF = swdMaxLookahead
	s.SwdThreshold = swdMinMatchLen

	s.MaxChain = swdDefaultMaxChain
	s.NiceLength = s.SwdF
	s.bufferSize = s.SwdN + s.SwdF
	s.bufferWrap = s.buffer[s.bufferSize:]
	s.nodeCount = s.SwdN

	s.insertPos = 0
	s.scanPos = s.insertPos
	s.firstRemovePos = s.insertPos
	if s.insertPos+s.SwdF > s.bufferSize {
		return ErrCompressInternal
	}

	s.Look = uint(len(s.compressor.input)) - s.insertPos
	if s.Look > 0 {
		if s.Look > s.SwdF {
			s.Look = s.SwdF
		}
		copy(s.buffer[s.insertPos:], s.compressor.input[:s.Look])
		s.compressor.inputPos += int(s.Look) //nolint:gosec // G115: Look bounded by SwdF
		s.insertPos += s.Look
	}

	if s.insertPos == s.bufferSize {
		s.insertPos = 0
	}

	s.removePos = s.firstRemovePos
	if s.removePos >= s.nodeCount {
		s.removePos -= s.nodeCount
	} else {
		s.removePos += s.bufferSize - s.nodeCount
	}

	if s.Look < 3 {
		s.buffer[s.scanPos+s.Look] = 0
		s.buffer[s.scanPos+s.Look+1] = 0
		s.buffer[s.scanPos+s.Look+2] = 0
	}

	return nil
}

// getByte gets the next byte from the input and adds it to the ring buffer.
func (s *slidingWindowDict) getByte() {
	if s.compressor.inputPos < len(s.compressor.input) {
		c := s.compressor.input[s.compressor.inputPos]
		s.compressor.inputPos++
		s.buffer[s.insertPos] = c
		if s.insertPos < s.SwdF {
			s.bufferWrap[s.insertPos] = c
		}
	} else {
		if s.Look > 0 {
			s.Look--
		}
		s.buffer[s.insertPos] = 0
		if s.insertPos < s.SwdF {
			s.bufferWrap[s.insertPos] = 0
		}
	}

	s.insertPos++
	if s.insertPos == s.bufferSize {
		s.insertPos = 0
	}
	s.scanPos++
	if s.scanPos == s.bufferSize {
		s.scanPos = 0
	}
	s.removePos++
	if s.removePos == s.bufferSize {
		s.removePos = 0
	}
}

// accept removes the first n bytes from the hash chains.
func (s *slidingWindowDict) accept(n uint) error {
	if n > s.Look {
		return ErrCompressInternal
	}

	for i := uint(0); i < n; i++ {
		if err := s.removeNode(s.removePos); err != nil {
			return err
		}

		key := head3(s.buffer[s.scanPos:])
		s.chainNext[s.scanPos] = s.headForKey3(key)
		s.hashHead3[key] = uint16(s.scanPos)           //nolint:gosec // G115: scanPos in window size
		s.chainBestLen[s.scanPos] = uint16(s.SwdF + 1) //nolint:gosec // G115: SwdF+1 fits uint16
		s.hashChainLen[key]++
		if uint(s.hashChainLen[key]) > s.SwdN {
			return ErrCompressInternal
		}

		key = head2(s.buffer[s.scanPos:])
		s.hashHead2[key] = uint16(s.scanPos + 1) //nolint:gosec // G115: scanPos in window size

		s.getByte()
	}
	return nil
}

// searchBestMatch scans a hash chain to improve the current best match.
func (s *slidingWindowDict) searchBestMatch(node uint, cnt uint) error {
	if s.MLen <= 0 {
		return ErrCompressInternal
	}

	buffer := s.buffer[:]
	currentMatchLen := s.MLen
	scanPos := s.scanPos
	scanLimit := scanPos + s.Look

	matchProbeByte := buffer[scanPos+currentMatchLen-1]
	for ; cnt > 0; cnt-- {
		if currentMatchLen >= s.Look {
			return ErrCompressInternal
		}

		if buffer[node+currentMatchLen-1] == matchProbeByte &&
			buffer[node+currentMatchLen] == buffer[scanPos+currentMatchLen] &&
			buffer[node] == buffer[scanPos] &&
			buffer[node+1] == buffer[scanPos+1] {
			matchedLen := uint(2)
			for scanPos+matchedLen < scanLimit && buffer[scanPos+matchedLen] == buffer[node+matchedLen] {
				matchedLen++
			}

			if matchedLen < swdBestOffCount && s.bestPos[matchedLen] == 0 {
				s.bestPos[matchedLen] = node + 1
			}
			if matchedLen > currentMatchLen {
				currentMatchLen = matchedLen
				s.MLen = currentMatchLen
				s.matchPos = node

				if currentMatchLen == s.Look {
					return nil
				}
				if currentMatchLen >= s.NiceLength {
					return nil
				}
				if currentMatchLen > uint(s.chainBestLen[node]) {
					return nil
				}

				matchProbeByte = buffer[scanPos+currentMatchLen-1]
			}
		}

		node = uint(s.chainNext[node])
	}

	return nil
}

// searchShortMatch searches for a 2-byte short match.
func (s *slidingWindowDict) searchShortMatch() (bool, error) {
	if s.Look < 2 {
		return false, ErrCompressInternal
	}
	if s.MLen <= 0 {
		return false, ErrCompressInternal
	}

	keyRaw := s.hashHead2[head2(s.buffer[s.scanPos:])]
	if keyRaw == 0 {
		return false, nil
	}
	key := uint(keyRaw - 1)

	if s.bestPos[2] == 0 {
		s.bestPos[2] = key + 1
	}
	if s.MLen < 2 {
		s.MLen = 2
		s.matchPos = key
	}

	return true, nil
}

// findBestMatch finds the best match in the current dictionary state.
func (s *slidingWindowDict) findBestMatch() error {
	if s.MLen == 0 {
		return ErrCompressInternal
	}

	key := head3(s.buffer[s.scanPos:])
	node := s.headForKey3(key)
	s.chainNext[s.scanPos] = node
	cnt := uint(s.hashChainLen[key])
	s.hashChainLen[key]++
	if cnt > s.SwdN+s.SwdF {
		return ErrCompressInternal
	}

	if cnt > s.MaxChain && s.MaxChain > 0 {
		cnt = s.MaxChain
	}
	s.hashHead3[key] = uint16(s.scanPos) //nolint:gosec // G115: scanPos in window size

	s.BChar = int(s.buffer[s.scanPos])
	mLen := s.MLen
	if s.MLen >= s.Look {
		if s.Look == 0 {
			s.BChar = -1
		}
		s.MOff = 0
		s.chainBestLen[s.scanPos] = uint16(s.SwdF + 1) //nolint:gosec // G115: SwdF+1 fits uint16
	} else {
		ok, err := s.searchShortMatch()
		if err != nil {
			return err
		}

		if ok && s.Look >= 3 {
			if err := s.searchBestMatch(uint(node), cnt); err != nil {
				return err
			}
		}

		if s.MLen > mLen {
			s.MOff = s.positionToOffset(s.matchPos)
		}

		if s.UseBestOff {
			for i := 2; i < swdBestOffCount; i++ {
				if s.bestPos[i] > 0 {
					s.BestOff[i] = s.positionToOffset(s.bestPos[i] - 1)
				} else {
					s.BestOff[i] = 0
				}
			}
		}
	}

	if err := s.removeNode(s.removePos); err != nil {
		return err
	}
	key = head2(s.buffer[s.scanPos:])
	s.hashHead2[key] = uint16(s.scanPos + 1) //nolint:gosec // G115: scanPos in window size
	return nil
}

// removeNode removes the node from the hash chains.
func (s *slidingWindowDict) removeNode(node uint) error {
	if s.nodeCount == 0 {
		key := head3(s.buffer[node:])
		if s.hashChainLen[key] == 0 {
			return ErrCompressInternal
		}
		s.hashChainLen[key]--

		key = head2(s.buffer[node:])
		if s.hashHead2[key] == 0 {
			return ErrCompressInternal
		}

		if uint(s.hashHead2[key]) == node+1 {
			s.hashHead2[key] = 0
		}
		return nil
	}

	s.nodeCount--
	return nil
}

// positionToOffset converts a dictionary position to a backward match offset.
func (s *slidingWindowDict) positionToOffset(pos uint) uint {
	if s.scanPos > pos {
		return s.scanPos - pos
	}
	return s.bufferSize - (pos - s.scanPos)
}
