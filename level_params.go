package lzo

// compressLevelParams holds internal parameters for one LZO1X-999 compression level.
// All fields are unexported; the type is used only inside the package.
type compressLevelParams struct {
	tryLazy  int    // try lazy matching (0/1/2)
	goodLen  uint   // good match length threshold
	maxLazy  uint   // max lazy match length
	niceLen  uint   // nice match length (stop searching)
	maxChain uint   // max hash chain length
	flags    uint32 // e.g. UseBestOff
}

// fixedLevels defines parameters for compression levels 1–9. Level 0/1 use fast LZO1X-1.
var fixedLevels = [9]compressLevelParams{
	{0, 0, 0, 8, 4, 0},
	{0, 0, 0, 16, 8, 0},
	{0, 0, 0, 32, 16, 0},
	{1, 4, 4, 16, 16, 0},
	{1, 8, 16, 32, 32, 0},
	{1, 8, 16, 128, 128, 0},
	{2, 8, 32, 128, 256, 0},
	{2, 32, 128, swdMaxLookahead, 2048, 1},
	{2, swdMaxLookahead, swdMaxLookahead, swdMaxLookahead, 4096, 1},
}
