// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/lzo

package lzo

import (
	"math"
	"slices"
)

// Encoder owns reusable LZO1X-999 compression state.
// The zero value is ready to use.
// An Encoder must not be copied after first use or used concurrently.
type Encoder struct {
	dict *hcCompressorDict
}

// NewEncoder allocates and retains a reusable LZO1X-999 dictionary until the
// returned Encoder becomes unreachable.
func NewEncoder() *Encoder {
	return &Encoder{dict: &hcCompressorDict{}}
}

// Compress compresses src with LZO1X. opts may be nil (uses default level 1).
// Level 0 or 1 = fast LZO1X-1; 2–9 = LZO1X-999 (better ratio, slower).
func Compress(src []byte, opts *CompressOptions) ([]byte, error) {
	if opts == nil {
		opts = DefaultCompressOptions()
	}
	level := max(opts.Level, 0)

	if level <= 1 {
		return compress1xFast(nil, src), nil
	}

	return compress999Level(src, min(level, 9))
}

// CompressInto compresses src into caller-provided dst and returns dst[:n].
// dst must have length of at least MaxCompressedSize(len(src)).
// src and dst must not overlap.
func CompressInto(src, dst []byte, opts *CompressOptions) ([]byte, error) {
	required := MaxCompressedSize(len(src))
	if required < 0 || len(dst) < required {
		return nil, ErrCompressBufferTooSmall
	}

	return appendCompress(dst[:0:required], src, opts)
}

// CompressInto compresses src into caller-provided dst using reusable encoder state.
// dst must have length of at least MaxCompressedSize(len(src)).
// src and dst must not overlap.
func (e *Encoder) CompressInto(src, dst []byte, opts *CompressOptions) ([]byte, error) {
	required := MaxCompressedSize(len(src))
	if required < 0 || len(dst) < required {
		return nil, ErrCompressBufferTooSmall
	}

	return e.appendCompress(dst[:0:required], src, opts)
}

// AppendCompress appends compressed src to dst and returns the extended slice.
// src and dst must not overlap.
func AppendCompress(dst, src []byte, opts *CompressOptions) ([]byte, error) {
	required := MaxCompressedSize(len(src))
	if required < 0 {
		return nil, ErrCompressBufferTooSmall
	}

	start := len(dst)
	dst = slices.Grow(dst, required)
	dst = dst[:start+required]

	out, err := appendCompress(dst[start:start:start+required], src, opts)
	if err != nil {
		return dst[:start], err
	}
	return dst[:start+len(out)], nil
}

// AppendCompress appends compressed src to dst using reusable encoder state.
// src and dst must not overlap.
func (e *Encoder) AppendCompress(dst, src []byte, opts *CompressOptions) ([]byte, error) {
	required := MaxCompressedSize(len(src))
	if required < 0 {
		return nil, ErrCompressBufferTooSmall
	}

	start := len(dst)
	dst = slices.Grow(dst, required)
	dst = dst[:start+required]

	out, err := e.appendCompress(dst[start:start:start+required], src, opts)
	if err != nil {
		return dst[:start], err
	}
	return dst[:start+len(out)], nil
}

// MaxCompressedSize returns the worst-case LZO stream size for an input length.
// It returns -1 when srcLen is negative or the result cannot fit in an int.
func MaxCompressedSize(srcLen int) int {
	if srcLen < 0 {
		return -1
	}

	size := srcLen + srcLen/16
	if size < srcLen || size > math.MaxInt-67 {
		return -1
	}
	return size + 67
}

func appendCompress(dst, src []byte, opts *CompressOptions) ([]byte, error) {
	if opts == nil {
		opts = DefaultCompressOptions()
	}
	level := opts.Level
	level = max(level, 0)

	if level <= 1 {
		return compress1xFast(dst, src), nil
	}

	level = min(level, 9)
	dict := acquireCompressorDict()
	defer releaseCompressorDict(dict)

	outLen, err := compress999NoAlloc(src, dst[:cap(dst)], dict, level)
	if err != nil {
		return nil, err
	}
	return dst[:outLen], nil
}

func (e *Encoder) appendCompress(dst, src []byte, opts *CompressOptions) ([]byte, error) {
	if opts == nil {
		opts = DefaultCompressOptions()
	}
	level := max(opts.Level, 0)
	if level <= 1 {
		return compress1xFast(dst, src), nil
	}

	if e.dict == nil {
		e.dict = &hcCompressorDict{}
	}

	outLen, err := compress999NoAlloc(src, dst[:cap(dst)], e.dict, min(level, 9))
	if err != nil {
		return nil, err
	}
	return dst[:outLen], nil
}

// opcodeByte packs an opcode fragment to one byte as required by LZO bit layout.
// Callers pass values whose low 8 bits are the serialized representation.
func opcodeByte(v int) byte {
	// #nosec G115 -- LZO opcodes intentionally encode only low 8 bits.
	return byte(v & 0xff)
}
