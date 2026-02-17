// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/lzo

package lzo

import (
	"math/bits"
	"sync"
	"unsafe"
)

const (
	// hcHashSize is the match-hash table size for the high-compression 1X-999 path.
	hcHashSize = 0x4000

	// hcMaxDist is the maximum back-reference distance in the compressor window.
	hcMaxDist = 0xbfff

	// hcMaxMatchLen is the maximum lookahead length considered by the matcher.
	hcMaxMatchLen = 0x800

	// hcBufferSize is the base sliding-window size.
	hcBufferSize = hcMaxDist + hcMaxMatchLen

	// hcBufferGuardSize is the window size plus wrap area used for contiguous comparisons.
	hcBufferGuardSize = hcBufferSize + hcMaxMatchLen

	// hcBestTableSize is the size of the "best offset by match length" lookup table.
	hcBestTableSize = maxLenM3 + 1

	// hcNilNode marks an empty hash-chain node.
	hcNilNode = 0xffff
)

// hcSearchDepthByLevel maps compression level (1..9) to hash-chain probe depth.
// Values are tuned empirically: higher depth improves ratio but increases CPU cost.
var hcSearchDepthByLevel = [10]int{
	0,   // 0 unused
	8,   // level 1
	12,  // level 2
	16,  // level 3
	24,  // level 4
	48,  // level 5
	64,  // level 6
	80,  // level 7
	96,  // level 8
	112, // level 9
}

// hcDictPool stores reusable compressor dictionaries to reduce allocations.
var hcDictPool = sync.Pool{
	New: func() any {
		return &hcCompressorDict{}
	},
}

// hcCompressBufferPool stores temporary encode buffers used by the 1X-999 path.
var hcCompressBufferPool sync.Pool

// hcCompressBuffer wraps reusable temporary output storage for the 1X-999 path.
type hcCompressBuffer struct {
	data []byte // data is the temporary encoded stream buffer.
}

// hcMatch3Table stores the 3-byte hash chains and per-node best-length cache.
type hcMatch3Table struct {
	head    [hcHashSize]uint16   // head is the newest node for each 3-byte hash key.
	chainSz [hcHashSize]uint16   // chainSz is the active node count for each hash key.
	chain   [hcBufferSize]uint16 // chain stores the previous node pointer for each ring position.
	slotKey [hcBufferSize]uint16 // slotKey stores the hash key used when a ring slot was inserted.
	bestLen [hcBufferSize]uint16 // bestLen caches the best match length found at each ring position.
}

// hcMatch2Table stores the short 2-byte match heads.
type hcMatch2Table struct {
	head [1 << 16]uint16 // head stores the last seen node+1 for each 2-byte key; 0 means empty.
}

// hcCompressorDict owns all mutable state for one compression run.
type hcCompressorDict struct {
	match3 hcMatch3Table           // match3 is the primary index for long matches.
	match2 hcMatch2Table           // match2 is the fallback index for very short matches.
	buffer [hcBufferGuardSize]byte // buffer is the ring window plus guard bytes for wrap-safe compare.
}

// hcState tracks the sliding input window and current scan positions.
type hcState struct {
	src []byte // src is the full input being compressed.

	inPos int // inPos is the next unread source byte index.

	windSize int // windSize is the current valid lookahead length from windB.
	windB    int // windB is the current ring position where parsing happens.
	windE    int // windE is the ring position where the next source byte is inserted.

	cycleCountdown int // cycleCountdown delays node eviction until the ring is fully primed.

	bufPos  int // bufPos is the absolute source position that maps to windB.
	bufSize int // bufSize is how many parse positions are still available in this step.
}

// Compress1X999Level compresses in with LZO1X-999 at the given level (1–9).
// Higher levels increase search depth and improve ratio at the cost of speed.
func Compress1X999Level(in []byte, level int) ([]byte, error) {
	return compress999Level(in, level)
}

// Compress1X999 compresses in with LZO1X-999 at level 9 (best ratio).
func Compress1X999(in []byte) ([]byte, error) {
	return compress999Level(in, 9)
}

// compress999Level is the MIT-based LZO1X-999 compressor used for levels 2..9.
func compress999Level(in []byte, level int) ([]byte, error) {
	if level < 1 {
		level = 1
	}
	if level > 9 {
		level = 9
	}

	dict := acquireCompressorDict()
	defer releaseCompressorDict(dict)

	temp := acquireCompressBuffer(maxCompressedSize(len(in)))
	defer releaseCompressBuffer(temp)

	outLen, err := compress999NoAlloc(in, temp.data, dict, level)
	if err != nil {
		return nil, err
	}

	out := make([]byte, outLen)
	copy(out, temp.data[:outLen])
	return out, nil
}

// compress999NoAlloc compresses in into out using the provided dictionary.
func compress999NoAlloc(in []byte, out []byte, dict *hcCompressorDict, level int) (int, error) {
	if len(out) < 3 {
		return 0, ErrCompressInternal
	}

	state := hcState{src: in}
	dict.init(&state)

	outPos := 0
	literalLen := 0
	// bestOffsetByLen caches alternative offsets discovered during search so
	// we can later shorten a chosen match if that yields a cheaper opcode.
	bestOffsetByLen := [hcBestTableSize]int{}
	literalStart := state.inPos
	searchDepth := hcSearchDepthByLevel[level]

	// Prime the parser with the first candidate match.
	matchOff, matchLen := dict.advance(&state, 0, &bestOffsetByLen, false, searchDepth)

	// Main parse loop: either extend a literal run or emit one back-reference token.
	for state.bufSize > 0 {
		if literalLen == 0 {
			literalStart = state.bufPos
		}

		// Filter out candidates that are valid as "matches" algorithmically but
		// cannot be emitted with legal LZO opcodes in the current stream context.
		if matchLen < 2 ||
			(matchLen == 2 && (matchOff > maxOffsetM1 || literalLen == 0 || literalLen >= 4)) ||
			(matchLen == 2 && outPos == 0) ||
			(outPos == 0 && literalLen == 0) {
			matchLen = 0
		} else if matchLen == minLenM2 && matchOff > maxOffsetMX && literalLen >= 4 {
			matchLen = 0
		}

		if matchLen == 0 {
			// No encodable match yet: grow literal run and try again at next position.
			literalLen++
			matchOff, matchLen = dict.advance(&state, 0, &bestOffsetByLen, false, searchDepth)
			continue
		}

		// Opcode cost is not monotonic in match length: sometimes a slightly
		// shorter match with smaller offset encodes to fewer bytes overall.
		findBetterMatch(bestOffsetByLen[:], &matchLen, &matchOff)

		if err := encodeLiteralRun(out, &outPos, in, literalStart, literalLen); err != nil {
			return 0, err
		}

		if err := encodeLookbackMatch(out, &outPos, matchLen, matchOff, literalLen); err != nil {
			return 0, err
		}

		prevLen := matchLen
		literalLen = 0
		matchOff, matchLen = dict.advance(&state, prevLen, &bestOffsetByLen, true, searchDepth)
	}

	if err := encodeLiteralRun(out, &outPos, in, literalStart, literalLen); err != nil {
		return 0, err
	}

	// Standard LZO end marker (M4 with distance class bit and zero payload).
	if err := writeByte(out, &outPos, markerM4|1); err != nil {
		return 0, err
	}
	if err := writeByte(out, &outPos, 0); err != nil {
		return 0, err
	}
	if err := writeByte(out, &outPos, 0); err != nil {
		return 0, err
	}

	return outPos, nil
}

// acquireCompressorDict returns a reusable high-compression dictionary.
func acquireCompressorDict() *hcCompressorDict {
	return hcDictPool.Get().(*hcCompressorDict)
}

// releaseCompressorDict returns a high-compression dictionary back to the pool.
func releaseCompressorDict(dict *hcCompressorDict) {
	if dict == nil {
		return
	}

	hcDictPool.Put(dict)
}

// acquireCompressBuffer returns a temporary output buffer wrapper with at least size bytes.
func acquireCompressBuffer(size int) *hcCompressBuffer {
	if buf, ok := hcCompressBufferPool.Get().(*hcCompressBuffer); ok {
		if cap(buf.data) >= size {
			buf.data = buf.data[:size]
			return buf
		}
	}

	// Allocate only when pool does not have enough capacity.
	return &hcCompressBuffer{data: make([]byte, size)}
}

// releaseCompressBuffer returns a temporary output buffer wrapper to the pool.
func releaseCompressBuffer(buf *hcCompressBuffer) {
	if buf == nil {
		return
	}

	// Keep capacity for reuse; logical length is reset by acquireCompressBuffer.
	buf.data = buf.data[:cap(buf.data)]
	hcCompressBufferPool.Put(buf)
}

// maxCompressedSize returns the worst-case output size for LZO streams.
func maxCompressedSize(inLen int) int {
	return inLen + inLen/16 + 64 + 3
}

// init prepares dictionary and state for a new compression run.
func (d *hcCompressorDict) init(state *hcState) {
	d.match3.init()
	d.match2.init()

	// Initialize the ring window with as much lookahead as available.
	state.cycleCountdown = hcMaxDist
	state.inPos = 0
	state.windSize = min(len(state.src), hcMaxMatchLen)
	state.windB = 0
	state.windE = state.windSize

	if state.windSize > 0 {
		copy(d.buffer[:state.windSize], state.src[:state.windSize])
	}
	state.inPos += state.windSize

	// Ensure first key derivation is always safe even on tiny inputs.
	if state.windSize < 3 {
		start := state.windB + state.windSize
		end := start + (3 - state.windSize)
		for i := start; i < end; i++ {
			d.buffer[i] = 0
		}
	}
}

// advance updates the dictionary window and returns the best current match.
func (d *hcCompressorDict) advance(state *hcState, prevLen int, bestOffsetByLen *[hcBestTableSize]int, skip bool, searchDepth int) (int, int) {
	// After emitting a match we still need to insert skipped bytes into both hash tables,
	// but we do not need to search from each of those intermediate positions.
	if skip && prevLen > 1 {
		for i := 0; i < prevLen-1; i++ {
			d.resetNextInputEntry(state)
			d.match3.skipAdvance(state, &d.buffer)
			state.getByte(&d.buffer)
		}
	}

	matchLen := 1
	matchOff := 0
	matchPos := 0
	bestPosByLen := [hcBestTableSize]int{}

	head, count := d.match3.advance(state, &d.buffer, searchDepth)
	if head == hcNilNode {
		count = 0
	}

	stop := false
	if matchLen >= state.windSize {
		// Reached lookahead end: no useful match can start here anymore.
		if state.windSize == 0 {
			stop = true
		}
		d.match3.bestLen[state.windB] = hcMaxMatchLen + 1 //nolint:gosec // G115: fixed 2049 fits uint16
	} else {
		// Search 3-byte hash chain candidates from newest to older positions.
		if state.windSize >= 3 {
			// Cheap 2-byte seed gives a baseline candidate before chain walk.
			d.match2.search(state, &matchPos, &matchLen, &bestPosByLen, &d.buffer)

			node := int(head)
			scanPos := state.windB
			scanLimit := scanPos + state.windSize
			currentBest := matchLen
			probeByte := d.buffer[scanPos+currentBest-1]

			// Walk the hash chain from newest to older candidates.
			for i := 0; i < count; i++ {
				if node < 0 || node >= hcBufferSize || node == int(hcNilNode) {
					break
				}

				if currentBest >= state.windSize {
					break
				}

				// Cheap pre-checks before the full byte-by-byte extension.
				if d.buffer[node+currentBest-1] != probeByte ||
					d.buffer[node+currentBest] != d.buffer[scanPos+currentBest] ||
					d.buffer[node] != d.buffer[scanPos] ||
					d.buffer[node+1] != d.buffer[scanPos+1] {
					next := d.match3.chain[node]
					if next == hcNilNode {
						break
					}
					node = int(next)
					continue
				}

				matched := countEqualBytes(&d.buffer, scanPos, node, 2, scanLimit)

				if matched >= 2 {
					// Remember the first position found for each length.
					if matched < hcBestTableSize && bestPosByLen[matched] == 0 {
						bestPosByLen[matched] = node + 1
					}

					if matched > matchLen {
						matchLen = matched
						matchPos = node
						currentBest = matched
						probeByte = d.buffer[scanPos+currentBest-1]

						// Early-stop heuristics:
						// 1) full lookahead match cannot be improved;
						// 2) cached bestLen says this node is unlikely to produce longer match.
						if matched == state.windSize || matched > int(d.match3.bestLen[node]) {
							break
						}
					}
				}

				next := d.match3.chain[node]
				if next == hcNilNode {
					break
				}
				node = int(next)
			}
		}

		if matchLen > 1 {
			matchOff = state.posToOffset(matchPos)
		}

		d.match3.bestLen[state.windB] = uint16(matchLen) //nolint:gosec // G115: bounded by window size
		for i := 2; i < hcBestTableSize; i++ {
			if bestPosByLen[i] > 0 {
				bestOffsetByLen[i] = state.posToOffset(bestPosByLen[i] - 1)
			} else {
				bestOffsetByLen[i] = 0
			}
		}
	}

	d.resetNextInputEntry(state)
	d.match2.add(state.windB, &d.buffer)
	state.getByte(&d.buffer)

	if stop {
		state.bufSize = 0
		matchLen = 0
	} else {
		state.bufSize = state.windSize + 1
	}
	state.bufPos = state.inPos - state.bufSize

	return matchOff, matchLen
}

// resetNextInputEntry removes stale hash entries before overwriting ring slots.
func (d *hcCompressorDict) resetNextInputEntry(state *hcState) {
	// Before the ring is fully primed there is nothing to evict yet.
	if state.cycleCountdown == 0 {
		d.match3.remove(state.windE)
	} else {
		state.cycleCountdown--
	}
}

// getByte advances state by one byte and maintains ring-window wrap bytes.
func (s *hcState) getByte(buffer *[hcBufferGuardSize]byte) {
	if s.inPos < len(s.src) {
		value := s.src[s.inPos]
		s.inPos++
		buffer[s.windE] = value

		// Mirror the prefix to the guard area so linear comparisons can cross the wrap.
		if s.windE < hcMaxMatchLen {
			buffer[hcBufferSize+s.windE] = value
		}
	} else {
		if s.windSize > 0 {
			s.windSize--
		}
		buffer[s.windE] = 0

		// Keep guard bytes coherent after input is exhausted.
		if s.windE < hcMaxMatchLen {
			buffer[hcBufferSize+s.windE] = 0
		}
	}

	s.windE++
	if s.windE == hcBufferSize {
		s.windE = 0
	}

	s.windB++
	if s.windB == hcBufferSize {
		s.windB = 0
	}
}

// posToOffset converts a ring-buffer position to a backward match distance.
func (s *hcState) posToOffset(pos int) int {
	if s.windB > pos {
		return s.windB - pos
	}
	return hcBufferSize - (pos - s.windB)
}

// init resets match3 chain sizes for a fresh compression run.
func (m *hcMatch3Table) init() {
	// Non-zero chainSz marks active keys for the current input.
	// Clearing 16K entries once per run is cheaper than checking key epochs on each step.
	clear(m.chainSz[:])
}

// remove removes a node from the 3-byte hash key count.
func (m *hcMatch3Table) remove(pos int) {
	// cycleCountdown guarantees eviction starts only after the ring is fully primed,
	// so slotKey[pos] is always initialized for positions we evict.
	key := int(m.slotKey[pos])
	// With one eviction per overwritten slot and one insertion per consumed byte,
	// chainSz for this key cannot underflow in valid stream flow.
	m.chainSz[key]--
}

// advance inserts current position into hash chains and returns chain head/count.
func (m *hcMatch3Table) advance(state *hcState, buffer *[hcBufferGuardSize]byte, searchDepth int) (uint16, int) {
	key := match3Key(buffer, state.windB)

	count := int(m.chainSz[key])
	// count gates candidate traversal; when count==0, head may hold stale value from
	// previous runs, but it is never dereferenced because search loop executes 0 steps.
	head := m.head[key]

	m.chain[state.windB] = head
	m.chainSz[key]++
	if count > hcMaxMatchLen {
		count = hcMaxMatchLen
	}
	if searchDepth > 0 && count > searchDepth {
		count = searchDepth
	}

	m.slotKey[state.windB] = uint16(key) //nolint:gosec // G115: key is bounded by hcHashSize (0x4000)
	m.head[key] = uint16(state.windB)    //nolint:gosec // G115: ring index always fits uint16
	return head, count
}

// skipAdvance inserts current position without searching for a match.
func (m *hcMatch3Table) skipAdvance(state *hcState, buffer *[hcBufferGuardSize]byte) {
	key := match3Key(buffer, state.windB)

	// Same rationale as in advance(): stale head is harmless while chainSz[key]==0.
	head := m.head[key]

	m.chain[state.windB] = head
	m.slotKey[state.windB] = uint16(key) //nolint:gosec // G115: key is bounded by hcHashSize (0x4000)
	m.head[key] = uint16(state.windB)    //nolint:gosec // G115: ring index always fits uint16
	m.bestLen[state.windB] = hcMaxMatchLen + 1
	m.chainSz[key]++
}

// init resets match2 head table.
func (m *hcMatch2Table) init() {
	clear(m.head[:])
}

// add stores current position for a 2-byte key.
func (m *hcMatch2Table) add(pos int, buffer *[hcBufferGuardSize]byte) {
	key := match2Key(buffer, pos)

	m.head[key] = uint16(pos + 1) //nolint:gosec // G115: ring index+1 fits uint16
}

// search tries to find a short 2-byte match at the current position.
// This is a low-cost seed; longer matches are still decided by match3 chain walk.
func (m *hcMatch2Table) search(state *hcState, matchPos *int, matchLen *int, bestPosByLen *[hcBestTableSize]int, buffer *[hcBufferGuardSize]byte) bool {
	key := match2Key(buffer, state.windB)

	head := m.head[key]
	if head == 0 {
		return false
	}

	pos := int(head) - 1

	if bestPosByLen[2] == 0 {
		bestPosByLen[2] = pos + 1
	}
	if *matchLen < 2 {
		*matchLen = 2
		*matchPos = pos
	}

	return true
}

// findBetterMatch applies LZO opcode-cost heuristics to shorten a chosen match
// when a nearby alternative yields smaller encoded size.
func findBetterMatch(bestOffsetByLen []int, matchLen *int, matchOff *int) {
	if *matchLen <= minLenM2 || *matchOff <= maxOffsetM2 {
		return
	}

	// Try L2 -> L1 reduction to fall into cheaper M2 distance class.
	if *matchOff > maxOffsetM2 && *matchLen >= minLenM2+1 && *matchLen <= maxLenM2+1 {
		shorterLen := *matchLen - 1
		shorterOff := bestOffsetAt(bestOffsetByLen, shorterLen)
		if shorterOff != 0 && shorterOff <= maxOffsetM2 {
			*matchLen = shorterLen
			*matchOff = shorterOff
			return
		}
	}

	// Try L2 -> L0 reduction for far matches that can become a compact M2.
	if *matchOff > maxOffsetM3 && *matchLen >= maxLenM4+1 && *matchLen <= maxLenM2+2 {
		shorterLen := *matchLen - 2
		shorterOff := bestOffsetAt(bestOffsetByLen, shorterLen)
		currentOff := bestOffsetAt(bestOffsetByLen, *matchLen)
		if shorterOff != 0 && currentOff <= maxOffsetM2 {
			*matchLen = shorterLen
			*matchOff = shorterOff
			return
		}
	}

	// Final fallback: reduce by one when it allows cheaper M3 form.
	if *matchOff > maxOffsetM3 && *matchLen >= maxLenM4+1 && *matchLen <= maxLenM3+1 {
		shorterLen := *matchLen - 1
		shorterOff := bestOffsetAt(bestOffsetByLen, shorterLen)
		shortestOff := bestOffsetAt(bestOffsetByLen, *matchLen-2)
		if shorterOff != 0 && shortestOff <= maxOffsetM3 {
			*matchLen = shorterLen
			*matchOff = shorterOff
		}
	}
}

// bestOffsetAt returns bestOffsetByLen[idx] when idx is in range.
func bestOffsetAt(bestOffsetByLen []int, idx int) int {
	if idx < 0 || idx >= len(bestOffsetByLen) {
		return 0
	}

	return bestOffsetByLen[idx]
}

// encodeLiteralRun writes one literal run and its length opcode.
func encodeLiteralRun(out []byte, outPos *int, in []byte, literalStart, literalLen int) error {
	if literalLen == 0 {
		return nil
	}

	switch {
	// First token can carry a compact literal-run prefix directly.
	case *outPos == 0 && literalLen <= 238:
		if err := writeByte(out, outPos, opcodeByte(17+literalLen)); err != nil {
			return err
		}

	// Very short literal runs are packed into low bits of the previous opcode.
	case literalLen <= 3:
		if *outPos < 2 {
			return ErrCompressInternal
		}
		out[*outPos-2] |= opcodeByte(literalLen)

	// Medium literal runs use one explicit length byte.
	case literalLen <= 18:
		if err := writeByte(out, outPos, opcodeByte(literalLen-3)); err != nil {
			return err
		}

	// Long literal runs use zero-extension encoding.
	default:
		if err := writeByte(out, outPos, 0); err != nil {
			return err
		}
		if err := writeZeroByteLength(out, outPos, literalLen-18); err != nil {
			return err
		}
	}

	return writeSlice(out, outPos, in[literalStart:literalStart+literalLen])
}

// encodeLookbackMatch writes one back-reference token.
func encodeLookbackMatch(out []byte, outPos *int, matchLen, matchOff, lastLiteralLen int) error {
	switch {
	// M1, 2-byte match, nearest distance class.
	case matchLen == 2:
		matchOff--
		if err := writeByte(out, outPos, opcodeByte(markerM1|((matchOff&0x3)<<2))); err != nil {
			return err
		}
		return writeByte(out, outPos, opcodeByte(matchOff>>2))

	// M2, short/medium distance class.
	case matchLen <= maxLenM2 && matchOff <= maxOffsetM2:
		matchOff--
		if err := writeByte(out, outPos, opcodeByte((matchLen-1)<<5|((matchOff&0x7)<<2))); err != nil {
			return err
		}
		return writeByte(out, outPos, opcodeByte(matchOff>>3))

	// M1 special case after >=4 literals (LZO opcode quirk).
	case matchLen == minLenM2 && matchOff <= maxOffsetMX && lastLiteralLen >= 4:
		matchOff -= 1 + maxOffsetM2
		if err := writeByte(out, outPos, opcodeByte(markerM1|((matchOff&0x3)<<2))); err != nil {
			return err
		}
		return writeByte(out, outPos, opcodeByte(matchOff>>2))

	// M3, longer match with medium distance.
	case matchOff <= maxOffsetM3:
		matchOff--
		if matchLen <= maxLenM3 {
			if err := writeByte(out, outPos, opcodeByte(markerM3|(matchLen-2))); err != nil {
				return err
			}
		} else {
			if err := writeByte(out, outPos, opcodeByte(markerM3)); err != nil {
				return err
			}
			if err := writeZeroByteLength(out, outPos, matchLen-maxLenM3); err != nil {
				return err
			}
		}

		if err := writeByte(out, outPos, opcodeByte((matchOff&0x3f)<<2)); err != nil {
			return err
		}
		return writeByte(out, outPos, opcodeByte(matchOff>>6))

	// M4, farthest distance class.
	case matchOff <= maxOffsetM4:
		matchOff -= 0x4000
		head := (matchOff & 0x4000) >> 11
		if matchLen <= maxLenM4 {
			if err := writeByte(out, outPos, opcodeByte(markerM4|head|(matchLen-2))); err != nil {
				return err
			}
		} else {
			if err := writeByte(out, outPos, opcodeByte(markerM4|head)); err != nil {
				return err
			}
			if err := writeZeroByteLength(out, outPos, matchLen-maxLenM4); err != nil {
				return err
			}
		}

		if err := writeByte(out, outPos, opcodeByte((matchOff&0x3f)<<2)); err != nil {
			return err
		}
		return writeByte(out, outPos, opcodeByte(matchOff>>6))
	}

	return ErrCompressInternal
}

// writeZeroByteLength writes long-length encoding as zero chunks plus tail.
func writeZeroByteLength(out []byte, outPos *int, length int) error {
	for length > 255 {
		if err := writeByte(out, outPos, 0); err != nil {
			return err
		}
		length -= 255
	}

	return writeByte(out, outPos, opcodeByte(length))
}

// writeByte writes one byte to out at *outPos.
func writeByte(out []byte, outPos *int, b byte) error {
	if *outPos >= len(out) {
		return ErrCompressInternal
	}

	out[*outPos] = b
	*outPos++
	return nil
}

// writeSlice writes data to out at *outPos.
func writeSlice(out []byte, outPos *int, data []byte) error {
	if len(data) > len(out)-*outPos {
		return ErrCompressInternal
	}

	copy(out[*outPos:*outPos+len(data)], data)
	*outPos += len(data)
	return nil
}

// countEqualBytes extends an already matched prefix and returns total match length.
func countEqualBytes(buffer *[hcBufferGuardSize]byte, leftPos, rightPos, matched, leftLimit int) int {
	// Use 8-byte words for the hot part of comparisons.
	// Unaligned loads are intentional here to reduce branchy byte loops.
	for leftPos+matched+8 <= leftLimit && rightPos+matched+8 <= hcBufferGuardSize {
		leftWord := *(*uint64)(unsafe.Pointer(&buffer[leftPos+matched]))
		rightWord := *(*uint64)(unsafe.Pointer(&buffer[rightPos+matched]))
		if leftWord == rightWord {
			matched += 8
			continue
		}

		diff := leftWord ^ rightWord
		matched += bits.TrailingZeros64(diff) >> 3
		return matched
	}

	// Finish the tail byte-by-byte.
	for leftPos+matched < leftLimit &&
		rightPos+matched < hcBufferGuardSize &&
		buffer[leftPos+matched] == buffer[rightPos+matched] {
		matched++
	}

	return matched
}

// match3Key computes the 3-byte hash key used by match3 chains.
func match3Key(buffer *[hcBufferGuardSize]byte, pos int) int {
	// One unaligned 32-bit load is cheaper than 3 separate byte loads in this hot path.
	v := *(*uint32)(unsafe.Pointer(&buffer[pos])) & 0x00ffffff
	return int((v * 0x1e35a7bd) >> (32 - 14))
}

// match2Key computes the 2-byte key used by short-match lookup.
func match2Key(buffer *[hcBufferGuardSize]byte, pos int) int {
	return int(buffer[pos]) ^ (int(buffer[pos+1]) << 8)
}
