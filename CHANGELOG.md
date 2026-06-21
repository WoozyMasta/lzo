<!-- markdownlint-disable MD024 -->
# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog][],
and this project adheres to [Semantic Versioning][].

<!--
## Unreleased

### Added
### Changed
### Removed
-->

## [0.3.2][] - 2026-06-21

### Changed

* Improved LZO1X-1 compression throughput by approximately 50%
  on highly repetitive inputs by eliminating bounds checks
  in the match-length extension inner loop.
* Improved LZO1X-1 compression throughput by approximately 10%
  on mixed compressible inputs.
* Improved decompressor throughput for streams with long matches
  by scanning zero-extension length bytes 8 at a time instead of one at a time.
* Reduced `Compress` heap allocations at level 0–1 from up to 16 to 1
  by routing through a pooled temporary buffer instead of growing from nil.

[0.3.2]: https://github.com/WoozyMasta/lzo/compare/v0.3.1...v0.3.2

## [0.3.1][] - 2026-06-21

### Fixed

* Fixed a stale compression match candidate after dictionary ring-slot reuse
  that could produce invalid LZO1X-999 output on some inputs.

[0.3.1]: https://github.com/WoozyMasta/lzo/compare/v0.3.0...v0.3.1

## [0.3.0][] - 2026-06-15

### Added

* Added `CompressInto`, `AppendCompress`, and `MaxCompressedSize`
  for caller-managed compression buffers.
* Added `DecompressFromReaderInto`
  for decoding reader input into caller-managed output memory.
* Added reusable `Encoder` state for deterministic allocation-free
  LZO1X-999 compression after initialization.

### Changed

* Improved LZO1X-1 compression throughput
  by approximately 44–80% on benchmarked compressible inputs.
* Improved LZO1X-999 compression throughput
  by approximately 25–33% on benchmarked mixed and incompressible inputs.
* Improved decompression throughput
  by approximately 15–17% on benchmarked token-heavy streams.
* Limited temporary memory retained after large LZO1X-999 compression calls.

### Fixed

* Enforced `DecompressFromReader` input limits while reading,
  preventing reads and allocations beyond `MaxInputSize`.

[0.3.0]: https://github.com/WoozyMasta/lzo/compare/v0.2.0...v0.3.0

## [0.2.0][] - 2026-02-17

### Added

* Added `DecompressInto` and `DecompressNInto`
  for zero-allocation decode into caller-provided buffers.

### Changed

* Switched project license to MIT.
* Reimplemented core logic from scratch based on MIT-licensed references
  (without copying GPL code).
* Improved compression and decompression throughput across benchmarked workloads.

[0.2.0]: https://github.com/WoozyMasta/lzo/compare/v0.1.3...v0.2.0

## [0.1.3][] - 2026-02-13

### Changed

* Refactoring code to reduce cognitive complexity and
  leverage modern programming techniques.

[0.1.3]: https://github.com/WoozyMasta/lzo/compare/v0.1.2...v0.1.3

## [0.1.2][] - 2026-02-10

### Added

* New function `DecompressN(bytes, opts)` returning decompressed data and
  number of input bytes consumed (`nRead`),
  for advancing over back-to-back compressed blocks.

[0.1.2]: https://github.com/WoozyMasta/lzo/compare/v0.1.1...v0.1.2

## [0.1.1][] - 2026-02-05

### Changed

* Replaced LZO decompressor with a single state-machine implementation

### Fixed

* M1/M2/M3 opcodes

[0.1.1]: https://github.com/WoozyMasta/lzo/compare/v0.1.0...v0.1.1

## [0.1.0][] - 2026-02-04

### Added

* First public release

[0.1.0]: https://github.com/WoozyMasta/lzo/tree/v0.1.0

<!--links-->
[Keep a Changelog]: https://keepachangelog.com/en/1.1.0/
[Semantic Versioning]: https://semver.org/spec/v2.0.0.html
