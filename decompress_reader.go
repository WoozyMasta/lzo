package lzo

import "io"

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
