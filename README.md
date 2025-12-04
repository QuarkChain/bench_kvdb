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

![ethereum.png](./images/ethereum.png)
This output shows detailed table and space statistics of a Pebble DB instance generated using: `pebble db properties /path-to-db`.

In this example, the database contains approximately `7.9 billion key-value` entries, stored across `14,840 SST` files 
distributed over levels L0–L6. The total on-disk data size is about `440 GB`.
For each level, the report breaks down:
- actual data block size (data)
- index block size (index)
- Bloom filter block size (filter)
- and raw key/value sizes.

From the size distribution:
- The majority of data resides in L6 (≈ 393 GB), which is expected for a healthy LSM-tree.
- Intermediate levels (L2–L5) exhibit an approximately 10× growth pattern, consistent with the typical LSM-tree level 
sizing strategy, though this is a soft target rather than a strict limit in Pebble.

**LBase Explanation**

In Pebble, **LBase is the base level currently selected by the compaction scheduler as the primary target for L0 compactions**.

LBase is **dynamic** and may change over time based on:

- compaction scores of each level
- write and flush pressure
- compaction backlog

In this snapshot:

- L1 is empty
- L2 contains about 54 MB of data

Therefore, **L2 is very likely acting as the current LBase**, meaning new L0 compactions will most likely target L2 directly.

### SST File Structure and Usage in PebbleDB

SST (Sorted String Table) is the core storage unit in PebbleDB. Each file contains multiple sections, each serving a specific purpose:

```
+-------------------+
|    Data Blocks    |  <- Actual key-value entries, read when Bloom filter/index indicate key exists
+-------------------+
|   Filter Block    |  <- Bloom filter, quickly checks if a key may exist, avoids unnecessary reads
+-------------------+
|   Index Blocks    |  <- Maps key ranges to data block offsets, helps locate data
+-------------------+
|  Top-Level Index  |  <- High-level index pointing to index blocks for faster lookup, read at iterator init
+-------------------+
|    Properties     |  <- File metadata (record count, global seq num)
+-------------------+
|    Meta-Index     |  <- Pointers to auxiliary data (filters, value blocks)
+-------------------+
|      Footer       |  <- Offsets for top-index and meta-index
+-------------------+
```

### Read Path in PebbleDB
A typical Get (key lookup) proceeds as follows: 
1. Check MemTable / Immutable MemTables (in memory - no I/O)

2. Consult MANIFEST metadata (in memory - no I/O)
    - Used to determine candidate SST files.
    - Already loaded in memory after DB open.

3. Search SST files (all levels)
    - L0: Multiple SSTs may need to be checked due to overlapping key ranges.
    - `LBase` and higher: SSTs have non-overlapping key ranges; at most one SST per level is queried.
    - Read the Top-Index at iterator init
      - Small structure at the end of each SST providing offsets to index blocks.
      - Usually cached; if not, 1 disk read. (~0 I/O)
    - Bloom Filter check
      - Table-level filter for the whole SST
      - If cached → no I/O
      - If not cached → 1 disk read to load the filter block
      - A high Bloom filter hit rate is crucial:
        If the filter says the key does not exist, the SST can be skipped entirely — meaning the lookup
        cost for a non-existing key is almost zero I/O.
    - If the Bloom Filter indicates the key may exist:
      - Index Block lookup → 0–1 I/O depending on cache state
      - Data Block read → 0–1 I/O depending on cache state
      - If found → return value.
    - Special optimization for the last level (L6)
      - L6 intentionally does not use Bloom filters.
      - The lookup directly uses the index block to locate the data block.
      - Reason for this optimization:
        - L6 contains the largest portion of the database.
        - The Bloom filter for L6 would be very large, with: 
          - Low cache efficiency
          - High memory cost
          - Diminishing filtering benefit (since most keys eventually fall into L6)
        - The index already provides efficient block-level pruning.
      - Therefore:
        - Skipping the L6 Bloom filter saves cache space
        - Avoids one extra disk read
        - Reduces total I/O and memory pressure

4. Continue downward through levels
    - Stop when the key is found, 
    - Or return `NotFound` after all candidate SSTs have been checked.


In simple terms
```
1. Check MemTable / Immutable MemTables
2. Check MANIFEST → candidate SSTs
3. For each SST:
   a) Table-level Bloom filter → skip SST if key absent
   b) Top-level index → find index block
   c) Index block → locate data block
   d) Data block → read value```
```

### Expected I/O Behavior

If the database has N non-empty levels (as shown in the example above. L0, L2, L3, L4, L5, L6 → N=6), 
The theoretical worst-case disk I/O count per Get operation is:

$$
\text{I/O} \approx (N - 1) + 2
$$

| Term | Meaning |
|------|---------|
| (N - 1) | Bloom filter loads for all levels except L6 |
| + 2 | Index block + Data block |

Which leads to the commonly quoted:
> Theoretical complexity: O(log N)

This reflects the fact that an LSM tree has logarithmic fan-out across levels.

### Why `O(log N)` Does Not Reflect Reality
In real blockchain workloads (e.g., Geth, Optimism), the theoretical `O(log N)` bound significantly overestimates real 
disk I/O, mainly due to the behavior of Bloom filters and cache residency.
1. Bloom Filters Prevent Most Useless I/O
    - Bloom filters are checked before index and data blocks.
    - If the filter says “key does not exist”, the SST is skipped with:
      - No index block read
      - No data block read
    - This effectively removes most negative lookups from touching disk.

2. Bloom Filters Excluding L6 Are Very Small
    - Typical Bloom filter size (for all levels except L6) + Top-Index size `≈ 0.2%` of total data size
    - Bloom filters:
      - Are accessed on almost every `Get`
      - Are highly cache-friendly
      - Have extremely high reuse
    - Therefore, if:  
      > Cache Size > ~0.2% of DB Size
    - then
      - Almost all Bloom filters remain resident in memory
      - Bloom filter lookups incur ~0 disk I/O
      - Top-Index block → 0 I/O
      - Combined with index and data block reads, the total I/O per Get operation tends toward 2 (1 for index, 1 for data block in worst case)

3. I/O Cost with Sufficient Cache
    - Cache can hold: `Bloom filters` + `All Index blocks` + `Hot data blocks` > `1.5%` of total data size
    - Then for most Get operations:
      - Bloom filter → 0 I/O
      - Top-Index block → 0 I/O
      - Index block → 0 I/O
    - So the real cost becomes:
      - `≈ 1–2 disk I/Os per Get` operation → Effectively O(1)
      - If cache is even larger or hot data blocks have high hit rate, I/O per Get operation can drop below 1

### Conclusion: PebbleDB Achieves O(1)-Like Read I/O Under Sufficient Cache
Although the theoretical read complexity of PebbleDB is `O(log N)` due to the multi-level LSM structure, 
this does not reflect real-world behavior. Thanks to: 
- The extremely small size of Bloom filters excluding L6 (~0.2% of DB),
- Their very high access frequency,
- And sufficient cache residency,

most negative lookups are filtered in memory, and positive lookups usually require only one or two data blocks read.

> With sufficient cache, the real read I/O complexity of PebbleDB is effectively O(1) and converges to 1–2 I/O per Get operation.


### Benchmark Plan: Validating the O(1)-Like Read Behavior

To verify the previous conclusion, the benchmark will:

1. Use **Pebble DB** (the same storage engine used by Geth).

2. Test multiple cache configurations:
    - `0.2%` and `0.4%` of DB size (Bloom-filter-dominant cache)
    - `1.5%` and `2%` of DB size (Bloom filter + index + hot data cache)

3. **Warm-up phase (to eliminate cold-cache effects):**
    - Before formal measurement, pre-read approximately **0.05% of the total key space**.
    - Purpose:
        - Populate Bloom filters and hot index blocks into cache,
        - Avoid inflated I/O caused by initial cold misses,
        - Ensure the system reaches a **steady-state cache behavior**.
    - Only after warm-up completes will formal statistics be collected.

4. Measure **storage I/O only**, not response latency.

5. Calculate **I/O per Get** using Pebble internal metrics, which closely approximate OS-level block I/O behavior:

   > I/O per Get ≈ (BlockCache Miss Count + TableCache Miss Count) / Key Lookup Count

6. Dataset sizes:
    - **Keys**: 32-byte hashes
        - 200M keys
        - 2B keys
        - 20B keys
    - **Values**: 110 bytes, matching the typical average value size in Ethereum trie storage.

This repository exists to **validate** that PebbleDB read I/O converges to **O(1)** under sufficient cache 
and steady-state access patterns.

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
- Pebble Version: v1.1.5

### IO per GET operation
Random-read benchmark using 10M random keys:

| Metric                                        | 200M Keys | 2B Keys  | 20B Keys |
|-----------------------------------------------|-----------|----------|----------|
| DB Size                                       | 22 GB     | 226 GB   | 2.2 TB   |
| Filter Size (Without L6)                      | 30 MB     | 273 MB   | 2.4 GB   |
| Filter Size (With L6)                         | 238 MB    | 2.3 GB   | 23 GB    |
| Index Size                                    | 176 MB    | 1.7 GB   | 18 GB    |
| IO per GET (Bloom Only, 0.2% DB Size)         | 32        | 6.46     | 8.64     |
| IO per GET (Bloom × 2, 0.4% DB Size)          | 1.02      | 1.94     | 5.14     |
| IO per GET (Bloom + Index, 1.5 % DB Size)     | 1.02      | 1.04     | 1.08     |
| IO per GET ((Bloom + Index) × 2, 3 % DB Size) | 1.02      | 1.04     | 1.08     |

Logs: [`src/bench_pebble/logs/`](https://github.com/QuarkChain/bench_kvdb/src/bench_pebble/logs/)
