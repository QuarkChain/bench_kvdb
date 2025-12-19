package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cockroachdb/pebble/sstable"
)

// Replace the following code to add Cache Hits detail
// https://github.com/cockroachdb/pebble/blob/v1.1.5/sstable/reader.go#L519

//var (
//	lock             sync.Mutex
//	CacheHitsDetail  = make(map[string]*atomic.Int32)
//	CacheCallsDetail = make(map[string]*atomic.Int32)
//)
//
//func countCacheHitsDetail(caller string) {
//	lock.Lock()
//	defer lock.Unlock()
//	if CacheHitsDetail[caller] == nil {
//		CacheHitsDetail[caller] = new(atomic.Int32)
//	}
//
//	CacheHitsDetail[caller].Add(1)
//}
//
//func countCacheCallsDetail(caller string) {
//	lock.Lock()
//	defer lock.Unlock()
//	if CacheCallsDetail[caller] == nil {
//		CacheCallsDetail[caller] = new(atomic.Int32)
//	}
//
//	CacheCallsDetail[caller].Add(1)
//}
//
//
//func (r *Reader) readBlock(
//	ctx context.Context,
//	bh BlockHandle,
//	transform blockTransform,
//	readHandle objstorage.ReadHandle,
//	stats *base.InternalIteratorStats,
//	bufferPool *BufferPool,
//) (handle bufferHandle, _ error) {
//	pc, _, _, ok := runtime.Caller(1)
//	caller := "unknown"
//	if ok {
//		caller = runtime.FuncForPC(pc).Name()
//	}
//	countCacheCallsDetail(caller)
//	if h := r.opts.Cache.Get(r.cacheID, r.fileNum, bh.Offset); h.Get() != nil {
//		countCacheHitsDetail(caller)

func FormatCacheStats() string {
	keys := make([]string, 0, len(sstable.CacheCallsDetail))
	for k := range sstable.CacheCallsDetail {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString("Cache Statistics:\n")
	sb.WriteString("--------------------------------------------------------\n")
	sb.WriteString("Caller\t\tHits\tMisses\tCalls\tHitRate\n")

	for _, k := range keys {
		hits := sstable.CacheHitsDetail[k]
		calls := sstable.CacheCallsDetail[k]

		hitsVal := int32(0)
		callsVal := int32(0)

		if hits != nil {
			hitsVal = hits.Load()
		}
		if calls != nil {
			callsVal = calls.Load()
		}

		hitRate := float64(0)
		if callsVal > 0 {
			hitRate = float64(hitsVal) / float64(callsVal)
		}

		sb.WriteString(fmt.Sprintf("%s\t%d\t%d\t%d\t%.2f\n", k, hitsVal, callsVal-hitsVal, callsVal, hitRate))
	}

	return sb.String()
}
