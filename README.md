# bench_kvdb

A benchmarking tool designed to evaluate random read performance of traditional KV databases such as **PebbleDB**, specifically focusing on **IO operations per key read (IO per GET)**.

This project originated from research around the new trie database design proposed in the Base `triedb` repo:

In the Base TrieDB repository (https://github.com/base/triedb/), the following design assumption is stated:

> “While traversing the trie from Root to Leaf in order to read a single value is predicted to scale logarithmically with 
> the size of the trie (O(log N)), this is also the cost associated with accessing each item stored in a Key/Value database.”

---

## Why This Project Exists

Many blockchain research discussions (like Base TrieDB) assume that KV DB (like pebble DB) requires approximately` O(log N)` work to perform a key lookup.
When this is combined with trie traversal (which also takes` O(log N)` steps), the **estimated cost becomes**: `O(log N * log N)`

However — this assumption may be **incorrect**.

### Why is Assumed to Have `log(N)` Lookup Cost? 

Take pebble as an example, the LSM-tree uses a level-based storage model. The base level (L1 or higher) is configured with a maximum 
size of 64 MB, and each subsequent level increases in capacity by a factor of 10, as shown in the pebble code below. 

```
https://github.com/cockroachdb/pebble/blob/master/options.go#L1536
o.LBaseMaxBytes = 64 << 20 // 64 MB

https://github.com/cockroachdb/pebble/blob/master/options.go#L736
// LevelMultiplier configures the size multiplier used to determine the
// desired size of each level of the LSM. Defaults to 10.
LevelMultiplier int
```
![sample.png](./images/sample.png)
### Read Path in PebbleDB
A typical Get(key) lookup proceeds as follows: 
1. Filter SST with MANIFEST metadata
  - Already resident in memory after DB open—no I/O.
2. Search Level 0 (L0)
  - Files may overlap in key ranges.
  - L0 is typically small and often fully cached, so lookups here usually incur `no disk I/O`.
  - All candidate SSTs for the key must be checked.
3. Search levels (LBase and higher)
  - Since levels from LBase onward have non-overlapping key ranges, at most one SST per level needs to be queried:
  - Bloom Filter check
    - If cached → no I/O
    - If not cached → 1 disk read to load the filter block
  - If the Bloom Filter indicates the key may exist:
    - Index Block lookup
      - 0–1 I/O depending on cache state
    - Data Block read
      - 0–1 I/O depending on cache state
    - Return value if the key is found.
  - If the Bloom Filter indicates the key does not exist:
    - Skip this SST immediately without reading index or data blocks.
4. Continue downward through levels
  - Stop when the key is found, or return NotFound after all candidate SSTs have been checked.

### Expected I/O Behavior

If the database has N non-empty levels (e.g. L0, L3, L4, L5, L6 → N=5), the theoretical worst-case number of disk 
reads is roughly: 
> I/O Count = N + 2

So it leads to theoretical O(log N) read complexity. 

### Why O(log N) Does Not Reflect Reality
In long-running blockchain workloads (e.g., Geth, Base), layers become warmed into cache. In many cases:
- Bloom filters of earlier levels are fully cached
- Index blocks may also remain in cache

This means the actual observed cost is often:
> < 2 I/O per Get

Instead of O(log N), real-world performance trends toward O(1) due to cache locality and repeated access patterns.

### Benchmark Plan 

To verify this actual behavior, the benchmark will:
1. Use Pebble DB (same DB used by Geth).
2. Test multiple cache configurations:
    - 16MB - min Geth cache size
    - 512MB - default Geth cache size
    - Full memory - Large enough cache to hold all Bloom filters + Index blocks.
3. Measure only storage I/O, not response latency. 
4. Calculate I/O per Get using `Pebble` internal metrics below Which is very similar to OS-level I/O: 
   > IO per GET ≈ (BlockCache Miss Count + TableCache Miss Count) / Key Lookup Count
5. Dataset sizes:
    - Keys: each key is a 32-byte hash
      - 200M keys
      - 2B keys
      - 20B keys
    - Value: 110 bytes, matching typical Ethereum average trie storage value size.

This repository exists to **empirically measure** that.

---

## Build & Run

### How to Build

```bash
git clone https://github.com/QuarkChain/bench_kvdb
cd bench_kvdb/src/bench_pebble
go build
```

### How to Run

**Usage：**
- --i：init insert data, default value is `false`
- --b: batch insert, default value is `true`
- --c: cache size in MB
- --T：total number of keys count
- --t: threads count
- --w：random write count
- --r：random read count
- --p：db path
- --l：log level


```bash

mkdir -p ./data


```

## Benchmark Results

### PebbleDB — IO per Random Read
Random-read benchmark using 10M random keys:
```
 Data Count    |  Size(MB)  |  IO per Key 
---------------+------------+--------------
   200M Keys   |   22 GB    |    1.01
   2B Keys     |  226 GB    |    1.92
   20B Keys    |  2.2 TB    |    2.5   
```

Logs: [`src/bench_pebble/runlog/`](https://github.com/QuarkChain/bench_kvdb/src/bench_pebble/runlog/)
