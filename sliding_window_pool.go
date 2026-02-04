package lzo

import "sync"

// slidingWindowDictPool is a pool of sliding window dictionaries.
var slidingWindowDictPool = sync.Pool{
	New: func() any {
		return &slidingWindowDict{}
	},
}

// acquireSlidingWindowDict acquires a sliding window dictionary from the pool.
func acquireSlidingWindowDict() *slidingWindowDict {
	dict := slidingWindowDictPool.Get().(*slidingWindowDict)
	*dict = slidingWindowDict{}
	return dict
}

// releaseSlidingWindowDict releases a sliding window dictionary to the pool.
func releaseSlidingWindowDict(dict *slidingWindowDict) {
	if dict == nil {
		return
	}

	dict.compressor = nil
	dict.bufferWrap = nil
	slidingWindowDictPool.Put(dict)
}
