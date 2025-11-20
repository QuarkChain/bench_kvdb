package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/bloom"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
)

const (
	valLen = 110
	keyLen = 32

	// minCache is the minimum amount of memory in megabytes to allocate to pebble
	// read and write caching, split half and half.
	minCache = 16

	// minHandles is the minimum number of files handles to allocate to the open
	// database files.
	minHandles = 16
)

var (
	ni       = flag.Bool("ni", false, "Need to insert kvs before test, default false")
	tc       = flag.Int64("T", 2000000000, "Number of kvs to insert before test, default value is 4_000_000_000")
	wc       = flag.Int64("w", 10000000, "Number of write count during the test")
	rc       = flag.Int64("r", 10000000, "Number of read count during the test")
	bi       = flag.Bool("bi", true, "Enable batch insert or not")
	fc       = flag.Bool("fc", false, "Force compact or not")
	t        = flag.Int64("t", 32, "Number of threads")
	dbPath   = flag.String("p", "./data/bench_pebble", "Data directory for the databases")
	logLevel = flag.Int64("l", 3, "Log level")
)

var (
	randBytes = make([]byte, valLen*keyLen)
)

func hash(sha crypto.KeccakState, in []byte) []byte {
	sha.Reset()
	sha.Write(in)
	h := make([]byte, 32)
	sha.Read(h)
	return h
}

func randomWrite(tid, count, start, end int64, db *pebble.DB, wg *sync.WaitGroup) {
	defer wg.Done()
	st := time.Now()
	key := make([]byte, keyLen)
	r := rand.New(rand.NewSource(time.Now().UnixNano() + tid))
	sha := crypto.NewKeccakState()
	for i := int64(0); i < count; i++ {
		rv := r.Int63n(end-start) + start
		s := (rv % keyLen) * valLen
		binary.BigEndian.PutUint64(key[keyLen-8:keyLen], uint64(rv))
		k := hash(sha, key)
		_ = db.Set(k, randBytes[s:s+valLen], nil)
		if *logLevel >= 3 && i%1000000 == 0 && i > 0 {
			ms := time.Since(st).Milliseconds()
			fmt.Printf("thread %d used time %d ms, hps %d\n", tid, ms, i*1000/ms)
		}
	}
	if *logLevel >= 3 {
		tu := time.Since(st).Seconds()
		fmt.Printf("thread %d random write done %.2fs, %.2f ops/s\n", tid, tu, float64(count)/tu)
	}
}

func randomRead(tid int64, keys [][]byte, db *pebble.DB, wg *sync.WaitGroup) {
	defer wg.Done()
	st := time.Now()
	i := int64(0)

	for _, key := range keys {
		v, _, e := db.Get(key)
		if *logLevel >= 3 && i%1000000 == 0 && i > 0 {
			fmt.Printf("value for key %v is %v with error %v\n", common.Bytes2Hex(key), v, e)
			ms := time.Since(st).Milliseconds()
			fmt.Printf("thread %d used time %d ms, hps %d\n", tid, ms, i*1000/ms)
		}
		i++
	}
	if *logLevel >= 3 {
		tu := time.Since(st).Seconds()
		fmt.Printf("thread %d random read done %.2fs, %.2f ops/s\n", tid, tu, float64(len(keys))/tu)
	}
}

func batchWrite(tid, count int64, db *pebble.DB, wg *sync.WaitGroup) {
	defer wg.Done()
	st := time.Now()
	sha := crypto.NewKeccakState()
	key := make([]byte, keyLen)
	batch := db.NewBatch()
	for i := int64(0); i < count; i++ {
		idx := tid*count + i
		s := (idx % keyLen) * valLen
		binary.BigEndian.PutUint64(key[keyLen-8:keyLen], uint64(idx))
		k := hash(sha, key)
		batch.Set(k, randBytes[s:s+valLen], nil)

		if i%1000 == 0 {
			_ = batch.Commit(pebble.NoSync)
			batch.Reset()
		} else {
			continue
		}
		if *logLevel >= 3 && i%1000000 == 0 && i > 0 {
			ms := time.Since(st).Milliseconds()
			fmt.Printf("thread %d used time %d ms, hps %d\n", tid, ms, i*1000/ms)
			fmt.Printf("sampel key %s\n", common.Bytes2Hex(k))
		}
	}
	_ = batch.Commit(pebble.NoSync)
	if *logLevel >= 3 {
		tu := time.Since(st).Seconds()
		fmt.Printf("thread %d batch write done %.2fs, %.2f ops/s\n", tid, tu, float64(count)/tu)
	}
}

func seqWrite(tid, count int64, db *pebble.DB, wg *sync.WaitGroup) {
	defer wg.Done()
	st := time.Now()
	key := make([]byte, keyLen)
	for i := int64(0); i < count; i++ {
		idx := tid*count + i
		s := (idx % keyLen) * valLen
		binary.BigEndian.PutUint64(key[keyLen-8:keyLen], uint64(idx))
		_ = db.Set(key, randBytes[s:s+valLen], nil)
		if *logLevel >= 3 && i%1000000 == 0 && i > 0 {
			ms := time.Since(st).Milliseconds()
			fmt.Printf("thread %d used time %d ms, hps %d\n", tid, ms, i*1000/ms)
		}
	}
	if *logLevel >= 3 {
		tu := time.Since(st).Seconds()
		fmt.Printf("thread %d seq write done %.2fs, %.2f ops/s\n", tid, tu, float64(count)/tu)
	}
}

func seqRead(tid, count int64, db *pebble.DB, wg *sync.WaitGroup) {
	defer wg.Done()
	st := time.Now()
	key := make([]byte, keyLen)
	for i := int64(0); i < count; i++ {
		binary.BigEndian.PutUint64(key[keyLen-8:keyLen], uint64(tid*count+i))
		_, _, _ = db.Get(key)
		if *logLevel >= 3 && i%1000000 == 0 && i > 0 {
			ms := time.Since(st).Microseconds()
			fmt.Printf("thread %d used time %d ms, hps %d\n", tid, ms, i*1000/ms)
		}
	}
	if *logLevel >= 3 {
		tu := time.Since(st).Seconds()
		fmt.Printf("thread %d seq read done %.2fs, %.2f ops/s\n", tid, tu, float64(count)/tu)
	}
}

func NewDB(file string, cache int, handles int, namespace string, readonly bool) (*pebble.DB, error) {
	// Ensure we have some minimal caching and file guarantees
	if cache < minCache {
		cache = minCache
	}
	if handles < minHandles {
		handles = minHandles
	}

	// The max memtable size is limited by the uint32 offsets stored in
	// internal/arenaskl.node, DeferredBatchOp, and flushableBatchEntry.
	//
	// - MaxUint32 on 64-bit platforms;
	// - MaxInt on 32-bit platforms.
	//
	// It is used when slices are limited to Uint32 on 64-bit platforms (the
	// length limit for slices is naturally MaxInt on 32-bit platforms).
	//
	// Taken from https://github.com/cockroachdb/pebble/blob/master/internal/constants/constants.go
	maxMemTableSize := (1<<31)<<(^uint(0)>>63) - 1

	// Two memory tables is configured which is identical to leveldb,
	// including a frozen memory table and another live one.
	memTableLimit := 2
	memTableSize := cache * 1024 * 1024 / 2 / memTableLimit

	// The memory table size is currently capped at maxMemTableSize-1 due to a
	// known bug in the pebble where maxMemTableSize is not recognized as a
	// valid size.
	//
	// TODO use the maxMemTableSize as the maximum table size once the issue
	// in pebble is fixed.
	if memTableSize >= maxMemTableSize {
		memTableSize = maxMemTableSize - 1
	}
	opt := &pebble.Options{
		// Pebble has a single combined cache area and the write
		// buffers are taken from this too. Assign all available
		// memory allowance for cache.
		Cache:        pebble.NewCache(int64(cache * 1024 * 1024)),
		MaxOpenFiles: handles,

		// The size of memory table(as well as the write buffer).
		// Note, there may have more than two memory tables in the system.
		MemTableSize: uint64(memTableSize),

		// MemTableStopWritesThreshold places a hard limit on the size
		// of the existent MemTables(including the frozen one).
		// Note, this must be the number of tables not the size of all memtables
		// according to https://github.com/cockroachdb/pebble/blob/master/options.go#L738-L742
		// and to https://github.com/cockroachdb/pebble/blob/master/db.go#L1892-L1903.
		MemTableStopWritesThreshold: memTableLimit,

		// The default compaction concurrency(1 thread),
		// Here use all available CPUs for faster compaction.
		MaxConcurrentCompactions: runtime.NumCPU,

		// Per-level options. Options for at least one level must be specified. The
		// options for the last level are used for all subsequent levels.
		Levels: []pebble.LevelOptions{
			{TargetFileSize: 2 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
			{TargetFileSize: 4 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
			{TargetFileSize: 8 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
			{TargetFileSize: 16 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
			{TargetFileSize: 32 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
			{TargetFileSize: 64 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
			{TargetFileSize: 128 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
		},
		ReadOnly: readonly,

		// Pebble is configured to use asynchronous write mode, meaning write operations
		// return as soon as the data is cached in memory, without waiting for the WAL
		// to be written. This mode offers better write performance but risks losing
		// recent writes if the application crashes or a power failure/system crash occurs.
		//
		// By setting the WALBytesPerSync, the cached WAL writes will be periodically
		// flushed at the background if the accumulated size exceeds this threshold.
		WALBytesPerSync: 5 * ethdb.IdealBatchSize,
	}
	// Disable seek compaction explicitly. Check https://github.com/ethereum/go-ethereum/pull/20130
	// for more details.
	opt.Experimental.ReadSamplingMultiplier = -1

	// Open the db and recover any potential corruptions
	db, err := pebble.Open(file, opt)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func main() {
	flag.Parse()

	db, err := NewDB(*dbPath, 512, 100000, "bench_pebble", false)

	if err != nil {
		log.Crit("New dashboard fail", "err", err)
	} else {
		defer db.Close()
	}

	threads := *t
	total := *tc
	writeCount := *wc
	readCount := *rc
	fmt.Printf("Threads: %d\n", threads)
	fmt.Printf("Total data: %d while needInit=%t\n", total, *ni)
	fmt.Printf("Ops: %d write ops and %d read ops\n", writeCount, readCount)

	rand.Read(randBytes)

	var wg sync.WaitGroup
	if *ni && total > 0 {
		start := time.Now()
		per := total / threads
		for tid := int64(0); tid < threads; tid++ {
			wg.Add(1)
			id := tid
			if *bi {
				go batchWrite(id, per, db, &wg)
			} else {
				go seqWrite(id, per, db, &wg)
			}
		}
		wg.Wait()
		ms := float64(time.Since(start).Milliseconds())
		fmt.Printf("Init write: %d ops in %.2f ms (%.2f ops/s)\n", total, ms, float64(total)*1000/ms)
		fmt.Printf("DB State \n%s", db.Metrics().String())
	}

	if *fc {
		if err = db.Compact([]byte{}, []byte{0xff, 0xff, 0xff, 0xff}, true); err != nil {
			panic(err)
		}
	}

	if writeCount > 0 {
		start := time.Now()
		per := writeCount / threads
		for tid := int64(0); tid < threads; tid++ {
			wg.Add(1)
			id := tid
			go randomWrite(id, per, 0, total, db, &wg)
		}
		wg.Wait()
		ms := float64(time.Since(start).Milliseconds())
		fmt.Printf("Random update: %d ops in %.2f ms (%.2f ops/s)\n", writeCount, ms, float64(writeCount)*1000/ms)
		fmt.Printf("DB State \n%s", db.Metrics().String())
	}

	if readCount > 0 {
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		thread_keys := make([][][]byte, threads)
		per := readCount / threads

		for i := int64(0); i < threads; i++ {
			keys := make([][]byte, per)
			sha := crypto.NewKeccakState()
			for j := int64(0); j < per; j++ {
				rv := r.Int63n(total)
				key := make([]byte, keyLen)
				binary.BigEndian.PutUint64(key[keyLen-8:keyLen], uint64(rv))
				k := hash(sha, key)
				keys[j] = k
			}
			thread_keys[i] = keys
		}

		m1 := db.Metrics()
		start := time.Now()
		for tid := int64(0); tid < threads; tid++ {
			wg.Add(1)
			id := tid
			keys := thread_keys[id]
			go randomRead(id, keys, db, &wg)
		}
		wg.Wait()
		ms := float64(time.Since(start).Milliseconds())
		fmt.Printf("Random read: %d ops in %.2f ms (%.2f ops/s)\n", readCount, ms, float64(readCount)*1000/ms)
		m2 := db.Metrics()

		amp := m2.ReadAmp()
		blockMiss := m2.BlockCache.Misses - m1.BlockCache.Misses
		tableMiss := m2.TableCache.Misses - m1.TableCache.Misses

		fmt.Printf("ReadAmp Count: %d\n", amp)
		fmt.Printf("BlockCache miss Count: %d\n", blockMiss)
		fmt.Printf("TableCache miss Count: %d\n", tableMiss)
		fmt.Printf("Avg I/O per Get ≈ %.4f\n",
			float64(blockMiss+tableMiss)/float64(readCount))

		for i := 0; i < 7; i++ {
			fmt.Printf("Level %d Bytes read: %.2f MB\n", i, float64(m2.Levels[i].BytesRead-m1.Levels[i].BytesRead)/1024/1024)
			fmt.Printf("Level %d Size: %.2f MB\n", i, float64(m2.Levels[i].Size)/1024/1024)
		}
		fmt.Printf("DB State \n%s", m2.String())
	}
}
