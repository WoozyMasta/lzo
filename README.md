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
