# lzo

Pure Go implementation of **LZO1X** compression and decompression,
compatible with `lzo1x_decompress_safe`-style streams.  
The current encoder/decoder cores were rewritten using permissive
[AxioDL/lzokay] (MIT) references.

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

Reuse caller-owned output memory:

```go
dst := make([]byte, lzo.MaxCompressedSize(len(data)))
compressed, err := lzo.CompressInto(data, dst, nil)

// Append to an existing slice and reuse its capacity when possible.
compressed, err = lzo.AppendCompress(dst[:0], data, nil)
```

`CompressInto` requires `len(dst) >= MaxCompressedSize(len(data))`.
The source and destination slices must not overlap.

For deterministic LZO1X-999 state reuse without relying on a shared pool:

```go
encoder := lzo.NewEncoder()
compressed, err := encoder.CompressInto(
    data,
    dst,
    &lzo.CompressOptions{Level: 9},
)
```

Each `Encoder` retains an LZO1X-999 dictionary.
It must not be copied after first use or used concurrently;
use one encoder per goroutine when needed.

### Decompress

`OutLen` (expected decompressed size)
is required for buffer allocation and safety.

```go
opts := lzo.DefaultDecompressOptions(expectedLen)
out, err := lzo.Decompress(compressed, opts)
```

`Decompress` returns `out[:n]`, so resulting length can be less than `OutLen`
when the stream terminator is reached earlier.

Need consumed input bytes for concatenated blocks:

```go
opts := lzo.DefaultDecompressOptions(expectedLen)
out, nRead, err := lzo.DecompressN(compressed, opts)
_ = out
compressed = compressed[nRead:]
```

Reuse caller-owned output buffer (no per-call output allocation):

```go
dst := make([]byte, expectedLen)
out, err := lzo.DecompressInto(compressed, dst)
out, nRead, err := lzo.DecompressNInto(compressed, dst)
_ = out
compressed = compressed[nRead:]
```

From an `io.Reader` (e.g. stream with known decompressed size):

```go
out, err := lzo.DecompressFromReader(r, lzo.DefaultDecompressOptions(expectedLen))

dst := make([]byte, expectedLen)
out, err = lzo.DecompressFromReaderInto(
    r,
    dst,
    lzo.DefaultDecompressOptions(expectedLen),
)
```

Optional limit on input size:

```go
opts := lzo.DefaultDecompressOptions(expectedLen)
opts.MaxInputSize = 1 << 20
out, err := lzo.DecompressFromReader(r, opts)
```

Reader APIs read the complete compressed stream before decoding.
`MaxInputSize` bounds the number of compressed bytes read and returns
`ErrInputTooLarge` when the limit is exceeded.

## Compression levels

| Level | Profile         | Engine     | Typical speed      | Typical ratio |
|-------|-----------------|------------|--------------------|---------------|
| 0     | Fastest         | LZO1X-1    | Fastest            | Good          |
| 1     | Fast (default)  | LZO1X-1    | Very fast          | Good          |
| 2–4   | Balanced        | LZO1X-999  | Slower than 0/1    | Better        |
| 5–9   | High-compress   | LZO1X-999  | Slowest            | Best          |

Higher levels (e.g. 9) give smaller output and are slower.

## Compatibility

* Output is LZO1X with match types M1–M4;
  stream ends with the standard terminator bytes `0x11 0x00 0x00`.
* Decompression is compatible with streams produced by
  `lzo1x_decompress_safe`-style encoders.

## Testing and benchmarks

```bash
make test
make bench
```

Benchmarks cover multiple inputs (small text, pattern, byte cycle)
and levels 1, 5, 9 for both compress and decompress.

[AxioDL/lzokay]: https://github.com/AxioDL/lzokay
