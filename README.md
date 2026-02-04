# lzo

Pure Go implementation of **LZO1X** compression and decompression,
compatible with `lzo1x_decompress_safe`-style streams.  

This project is based on [rasky/go-lzo] (GPLv2).
The code has been largely rewritten; see [What changed](#what-changed) below.

**Not a drop-in replacement** — API differs:
`Compress(data, opts)` and `Decompress(data, opts)` with required `OutLen`.

## Installation

```bash
go get github.com/woozymasta/lzo
```

## Usage

### Compress

`opts` may be `nil` (default level 1).
Level 0 or 1 = fast LZO1X-1; 2–9 = LZO1X-999.

```go
import "github.com/woozymasta/lzo"

// Default (level 1, fast)
compressed, err := lzo.Compress(data, nil)

// Best ratio (level 9)
compressed, err := lzo.Compress(data, &lzo.CompressOptions{Level: 9})

// Direct LZO1X-999 API
compressed, err := lzo.Compress1X999(data)       // level 9
compressed, err := lzo.Compress1X999Level(data, 5) // level 5
```

### Decompress

`OutLen` (expected decompressed size)
is required for buffer allocation and safety.

```go
opts := lzo.DefaultDecompressOptions(expectedLen)
out, err := lzo.Decompress(compressed, opts)
```

From an `io.Reader` (e.g. stream with known decompressed size):

```go
out, err := lzo.DecompressFromReader(r, lzo.DefaultDecompressOptions(expectedLen))
```

Optional limit on input size:

```go
opts := lzo.DefaultDecompressOptions(expectedLen)
opts.MaxInputSize = 1 << 20
out, err := lzo.DecompressFromReader(r, opts)
```

## Compression levels

| Level | Algorithm   | Speed   | Ratio  |
|-------|-------------|---------|--------|
| 0, 1  | LZO1X-1     | Fastest | Good   |
| 2–9   | LZO1X-999   | Slower  | Better |

Higher levels (e.g. 9) give smaller output and are slower.

## Compatibility

* Output is LZO1X with match types M1–M4;
  stream ends with the standard terminator (distance `0x4000`, length 1).
* Decompression is compatible with streams produced by
  `lzo1x_decompress_safe`-style encoders.

## What changed

Compared to [rasky/go-lzo]:

* **Error handling** — No `panic`;
  all failures return `error` with sentinel values (`errors.Is`).
  Compressor and sliding-window use `error` returns;
  decoder uses explicit errors
  (e.g. `ErrUnexpectedEOF` when stream ends without terminator).
* **API** —
  Unified `Compress(src, opts)` and `Decompress(src, opts)` with options;
  decompression requires `OutLen` via `DecompressOptions`;
  `DecompressFromReader` with optional `MaxInputSize`.
  No custom reader with Rebuffer.
* **Control flow** — Fast compressor (LZO1X-1) refactored to avoid `goto`;
  structured loops and helpers (e.g. `findCandidate`).
* **Format / decoder** —
  Initial literal run encoded as `17+count` so decoder stays in sync;
  decoder only succeeds on explicit terminator (no success on EOF without it).
* **Performance** — Pool for sliding-window dictionaries (`sync.Pool`)
  to reduce allocations in LZO1X-999.
* **Tests** —
  Round-trip tests across levels, truncated-input and edge-case tests,
  benchmarks for multiple inputs and levels;

## Testing and benchmarks

```bash
go test ./...
go test -bench=. ./...
```

Benchmarks cover multiple inputs (small text, pattern, byte cycle)
and levels 1, 5, 9 for both compress and decompress.

[rasky/go-lzo]: https://github.com/rasky/go-lzo
