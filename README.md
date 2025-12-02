# bench_kvdb

A benchmarking tool designed to evaluate random read performance of traditional KV databases such as **PebbleDB**, specifically focusing on **IO operations per key read (IO per GET operation)**.

This project originated from research around the new trie database design proposed in the Base `triedb` repo.
In the Base TrieDB repository (https://github.com/base/triedb/), the following design assumption is stated:

> While traversing the trie from Root to Leaf in order to read a single value is predicted to scale logarithmically with 
> the size of the trie (`O(log N)`), this is also the cost associated with accessing each item stored in a Key/Value database.
> In effect, the database must be fully searched for each independent trie node, and
> this work must be repeated until a Leaf node is found, resulting in a true scaling factor of O(log N * log N).

---

## Why This Project Exists

Many blockchain research discussions (like Base TrieDB) assume that KV DB (like pebble DB) requires approximately `O(log N)` work to perform a key lookup.

However, this assumption may be **incorrect** in the actual scenarios of blockchain.

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
This output shows Pebble DB statistics generated using `pebble db properties /path-to-db`.

In this example, the database contains 600m records, stored in 3,827 SST files distributed across levels L0–L6. 
The report includes per-level details such as the size of data blocks, index blocks, and Bloom filter blocks.
The `LBase` level is L3, which holds 62 MB of data, close to its configured limit of 64 MB. 
Beyond L3, each level is allowed to grow up to roughly 10× the capacity of the previous level, 
following the typical LSM-tree size amplification model.

Note: `LBase` is the first layer besides L0 that contains data.

### Read Path in PebbleDB
A typical Get (key lookup) proceeds as follows: 
1. Check MemTable / Immutable MemTables (in memory - no I/O)
2. Consult MANIFEST metadata
  - Used to determine candidate SST files;
  - Already loaded in memory after DB open — no I/O.
3. Search Level 0 (L0)
  - May need to check multiple SSTs because key ranges can overlap.
  - L0 is usually small and often fully cached, so lookups here usually incur `no disk I/O`.
4. Search levels (`LBase` and higher) 
  - Since levels from `LBase` onward have non-overlapping key ranges, at most one SST per level needs to be queried:
  - Bloom Filter check
    - If cached → no I/O
    - If not cached → 1 disk read to load the filter block
    - A high Bloom filter hit rate is crucial:
      If the filter says the key does not exist, the SST can be skipped entirely — meaning the lookup
      cost for a non-existing key is almost zero I/O.
  - If the Bloom Filter indicates the key may exist:
    - Index Block lookup → 0–1 I/O depending on cache state
    - Data Block read → 0–1 I/O depending on cache state
    - If found → return value.
  - If the Bloom Filter indicates the key does not exist:
    - Skip this SST immediately without reading index or data blocks.
5. Continue downward through levels
  - Stop when the key is found, 
  - Or return `NotFound` after all candidate SSTs have been checked.

### Expected I/O Behavior

If the database has N non-empty levels (as shown in the example above. L0, L3, L4, L5, L6 → N=5), the theoretical 
worst-case number of disk reads is roughly: 
> I/O Count = N (load bloom filter block) + 2 (load index block and data block)

So it leads to theoretical `O(log N)` read complexity. 

### Why `O(log N)` Does Not Reflect Reality
In long-running blockchain workloads (e.g., Geth, Optimism), frequently accessed metadata tend to remain in cache over time.  
When the cache is sufficiently large, the following components often become resident in memory:
- Bloom filters (especially for the hot or upper levels in the LSM tree)
- Index blocks
- In some cases, data blocks as well

This means the actual observed cost is often:
> < 2 I/O per Key

Instead of `O(log N)`, real-world performance trends toward `O(1)` due to cache state and repeated access patterns.

### Benchmark Plan 

To verify this actual behavior, the benchmark will:
1. Use Pebble DB (same DB used by Geth);
2. Test multiple cache configurations:
    - 16MB - Minimum Geth cache size;
    - 512MB - Default Geth cache size;
    - Large memory - Large enough cache to hold all Bloom filters + Index blocks.
3. Measure only storage I/O, not response latency;
4. Calculate I/O per Key using `Pebble` internal metrics below which is very similar to OS-level I/O: 
   > IO per GET operation ≈ (BlockCache Miss Count + TableCache Miss Count) / Key Lookup Count
5. Dataset sizes:
    - Keys: 32 bytes hash: 
      - 200M keys
      - 2B keys
      - 20B keys
    - Values: 110 bytes, matching typical Ethereum average trie storage value size.

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
- --w：random update count
- --r：random read count
- --p：db path
- --l：log level


```bash
cd ./src/bench_pebble

# init DB 
./run_insert.sh

# get db properties
./run_properties.sh

# run bench
./run_bench.sh
```

## Benchmark Results

### Environment
- CPU: 32 cores
- Memory: 128 GB
- Disk: SAMSUNG MZQL23T8HCLS-00A07 + RAID 0
- OS: Ubuntu

### IO per GET operation
Random-read benchmark using 10M random keys:

| Data Count   | DB Size | Filter Size | Index Size | IO per GET (16M) | IO per GET (512M) | IO per GET (large) |
|--------------|---------|-------------|------------|------------------|-------------------|--------------------|
| 200M Keys    | 22 GB   | 238 MB      | 176 MB     | 3.86             | 1.02              | 1.02 (512MB)       |
| 2B Keys      | 226 GB  | 2.3 GB      | 1.7 GB     | 6.46             | 1.94              | 1.04 (5.12GB)      |
| 20B Keys     | 2.2 TB  | 23 GB       | 18 GB      | 8.64             | 5.14              | 1.08 (51.2GB)      |

Logs: [`src/bench_pebble/logs/`](https://github.com/QuarkChain/bench_kvdb/src/bench_pebble/logs/)
