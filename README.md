# bench_kvdb

### Abstract

Modern blockchain systems rely on large-scale key-value (KV) databases to serve highly random state accesses at billions of keys. While the theoretical read complexity of LSM-based storage engines such as Pebble is often described as `O(log N)`, this model does not accurately reflect real disk I/O behavior under realistic cache conditions.

This work presents an extensive empirical study on the **true disk I/O cost of random KV lookups at blockchain scale**, using Pebble as the underlying storage engine. We benchmark databases ranging from **22 GB to 2.2 TB** (200M to 20B keys) under multiple cache configurations that selectively fit Bloom filters, top indexes, and full index blocks into memory.

Our experiments show that:
- Once **Bloom filters (excluding LLast) and Top-Index blocks** fit in cache, **most negative lookups incur zero disk I/O**, and the I/O per Get rapidly drops to ~2.
- When **all index blocks also fit in cache**, the I/O per Get further converges to **~1.0–1.3**, largely independent of total database size.
- Data block caching has only a secondary effect on overall I/O.

These results demonstrate that, under sufficient cache, **Pebble exhibits effectively O(1) disk I/O behavior for random reads**, challenging the common assumption that each KV lookup inherently costs `O(log N)` physical I/Os. This has direct implications for the performance modeling and design of blockchain trie databases and execution-layer storage systems.

---

### Overview

`bench_kvdb` is a benchmarking tool for measuring the **disk I/O cost of random key lookups** in key-value databases 
like Pebble, using **I/O operations per Get (I/O per Get)**.

Designed for blockchain-scale workloads, the tool simulates:
- Cryptographic hash keys,
- Highly random reads,
- Databases from tens of gigabytes to multiple terabytes.

This metric measures how efficiently a KV database translates logical lookups into physical storage access, which is 
critical for blockchain workloads where:
- Data scales to billions of keys,
- Reads are random and non-local,
- Cache behavior plays a dominant role.

The benchmark evaluates how Bloom filters, index blocks, and cache residency impact real I/O performance under various 
database sizes and cache configurations. The focus is on measuring real-world I/O behavior rather than theoretical complexity.


---

### Motivation

This project originated from research on the new trie database design proposed in
[Base/TrieDB](https://github.com/base/triedb/) repository, which assumes:

> Trie traversal costs `O(log N)`, and each underlying KV lookup also costs `O(log N)`, leading to 
> an overall read complexity of `O(log N × log N)`.

This model implicitly assumes that each KV lookup performs a logarithmic number of disk I/Os.
However, in real blockchain systems, these assumptions often do not hold.
Modern LSM-based KV engines such as Pebble rely heavily on:
- Bloom filters that eliminate most negative lookups without touching disk,
- Small, highly reusable index structures,
- Large block caches and OS page caches that keep critical metadata permanently resident in memory.

As a result, the real physical I/O behavior of a KV lookup is often:
- Zero disk I/O for most negative lookups,
- Only 1–2 disk I/Os for most positive lookups,
- And largely independent of total database size once Bloom filters and index blocks fit in cache.

Therefore, the key question that motivates this project is:

> At blockchain scale, what is the true disk I/O cost of a random KV lookup in practice?

This repository was created to answer this question with direct, empirical measurements, in order to:
- Validate or challenge the common O(log N) K V lookup assumption,
- Quantify how much cache is actually required to achieve near-constant read I/O,
- And provide a solid performance foundation for the design of trie databases, execution layers, and storage layers in blockchain systems.

### Pebble’s Level Structure 

Pebble uses a level-based LSM design where the base level is capped at 64 MB, and each subsequent level increases 
in capacity by a factor of 10.

![ethereum.png](./images/ethereum.png)

The example shown above contains 7.9 billion key–value pairs stored across `14,840 SST` files distributed over levels L2–L6, 
with most data concentrated in L6 (≈393 GB). Intermediate levels follow the expected ~10× growth pattern, 
though Pebble treats this as a soft target rather than a strict limit.

Pebble also maintains a dynamic **LBase** (Base Level), which is the base level currently selected by the compaction 
scheduler as the primary target for L0 compactions. The LBase is determined based on the sizes of the levels and the 
compaction pressure. In this snapshot, L1 is empty and L2 holds only ~54 MB, so **L2 is effectively the current LBase**.

Pebble also maintains a dynamic **LLast** (Last Level) refers to the level with the largest data size. 
Initially, L6 is the LLast level until the database exceeds its 6TB capacity (according to Pebble config). When this happens, 
new levels such as L7, L8, and so on are introduced, and the LLast level shifts to the highest available level.

### SST File Structure and Usage in Pebble

SST (Sorted String Table) is the core storage unit in Pebble. Each file contains multiple sections, each serving a specific purpose:

```
+-------------------+
|    Data Blocks    |  <- Actual key-value entries, read when Bloom filter/index indicates key exists
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

### Read Path in Pebble
A typical Get (key lookup) proceeds as follows: 
1. Check MemTable / Immutable MemTables (in memory - no I/O)

2. Consult MANIFEST metadata (in memory - no I/O)
    - Used to determine candidate SST files.
    - Already loaded in memory after DB open.

3. Search SST files (all levels)
    - L0: Multiple SSTs may need to be checked due to overlapping key ranges.
    - `LBase` and higher: SSTs have non-overlapping key ranges; at most one SST per level is queried.
      
   For each SST file
    - Read the Top-Index at iterator init
      - Small structure at the end of each SST providing offsets to index blocks.
      - Usually cached; if not, 1 disk read. (~0 I/O)
    - **Bloom Filter check**
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
    - Special optimization for the last level (LLast)
      - LLast intentionally does not use Bloom filters.
      - The lookup directly uses the index block to locate the data block.
      - Reason for this optimization:
        - LLast contains the largest portion of the database.
        - The Bloom filter for LLast would be very large, with: 
          - Low cache efficiency
          - High memory cost
          - Diminishing filtering benefit (since most keys eventually fall into LLast)
        - The index already provides efficient block-level pruning.
      - Therefore:
        - Skipping the LLast Bloom filter saves cache space
        - Avoids one extra disk read
        - Reduces total I/O and memory pressure

4. Continue downward through levels
    - Stop when the key is found, 
    - Or return `NotFound` after all candidate SSTs have been checked.


In simple terms:
```
1. Check MemTable / Immutable MemTables
2. Check MANIFEST → candidate SSTs
3. For each SST:
   a) Table-level Bloom filter → skip SST if key absent
   b) Top-level index → find index block
   c) Index block → locate data block
   d) Data block → read value
```

### Theoretical I/O Behavior

If the database has X non-empty levels (as shown in the example above, L2, L3, L4, L5, L6 → X=5), 
The theoretical worst-case disk I/O count per Get operation is:

$$
\text{I/O} \approx (X - 1) * 2 + 3
$$

| Term    | Meaning                                    |
|---------|--------------------------------------------|
| (X - 1) | All levels except LLast                    |
| * 2     | Top Index block + Filter block             |
| + 3     | Top Index block + Index block + Data block |

Which leads to the commonly quoted:
> Theoretical complexity: O(log N)

This reflects the fact that an LSM tree has logarithmic fan-out across levels.

### Why `O(log N)` Does Not Reflect Reality
In real blockchain workloads (e.g., Geth, Optimism), the theoretical `O(log N)` bound significantly overestimates real 
disk I/O, mainly due to the behavior of Bloom filters and cache residency.
1. Bloom Filters Prevent Most Useless I/O
    - Bloom filters are checked before the index and data blocks.
    - If the filter says “key does not exist”, the SST is skipped with:
      - No index block read
      - No data block read
    - This effectively removes most negative lookups from touching disk.

2. Bloom Filters Excluding LLast + Top-Index Are Small
    - Typical Bloom filter size (for all levels except LLast) + Top-Index size (`≈ 0.18%` of total data size in the Ethereum sample)
    - Bloom filters and Top-Index:
      - Are accessed on almost every `Get`
      - Are highly cache-friendly
      - Have extremely high reuse
    - Therefore, if:  
      > Cache Size > Bloom filter size without LLast + Top-Index size
    - then
      - Almost all Bloom filters and Top-Indexes remain resident in memory
      - Bloom filter lookups incur ~0 I/O
      - Top-Index block → 0 I/O
      - Combined with index and data block reads, the total I/O per Get operation tends toward 2 (1 for index, 1 for data block)

3. I/O Cost with Sufficient Cache
    - Cache can hold: `Bloom filters` + `All Index blocks` (`≈ 1.3%` of total data size in the Ethereum sample)
    - Then for most Get operations:
      - Bloom filter → 0 I/O
      - Top-Index block → 0 I/O
      - Index block → 0 I/O
    - So the real cost becomes:
      - `≈ 1–2 I/Os per Get` operation → Effectively O(1)
      - If the cache is even larger and hot data blocks are cached and have a high hit rate, I/O per Get operation can drop below 1

### Hypothesis: Pebble Achieves O(1)-Like Read I/O Under Sufficient Cache
Although the theoretical read complexity of Pebble is `O(log N)` due to the multi-level LSM structure, 
This does not reflect real-world behavior. Thanks to: 
- The small size of Bloom filters, excluding LLast,
- Their very high access frequency,
- And sufficient cache residency,

Most negative lookups are filtered out in memory, and positive lookups usually require only one or two data blocks read.

We hypothesize that
> With sufficient cache, the real read I/O complexity of Pebble is effectively O(1) and converges to 1–2 I/O per Get operation.


### Benchmark Plan: Validating the O(1)-Like Read Behavior

To verify the previous hypothesis, the benchmark will:

1. Use **Pebble** (the same storage engine used by Geth).

2. Test multiple cache configurations:
    - From 0.1% of the DB Size, which is smaller than `Filter (without LLast) + Top Index`
    - To 3% of the DB Size, which is larger than `Filter (without LLast) + All Index`

3. **Warm-up phase (to eliminate cold-cache effects):**
    - Before formal measurement, pre-read approximately **0.05% of the total key space**.
    - Purpose:
        - Populate Bloom filters, Top-index blocks, and hot index blocks into cache,
        - Ensure the system reaches a **steady-state cache behavior**.
    - Only after the warm-up completes will formal statistics be collected.

4. Measure **storage I/O only**, not response latency.

5. Calculate **I/O per Get** using Pebble internal metrics, which closely approximate OS-level I/O behavior:

   > I/O per Get ≈ (BlockCache Miss Count + TableCache Miss Count) / Key Lookup Count

6. Dataset sizes:
    - Records:
        - Small DB: 200M keys
        - Medium DB: 2B keys
        - Large DB: 20B keys
    - Keys: 32-byte hashes
    - Values: 110 bytes, matching the typical average value size in Ethereum trie storage.

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
- --i: init insert data, default value is `false`
- --b: batch insert, default value is `true`
- --c: cache size in MB
- --T: total number of keys count
- --t: threads count
- --w: random update count
- --r: random read count
- --p: db path
- --l: log level


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
- Disk: 7 TB (2 * SAMSUNG MZQL23T8HCLS-00A07 + RAID 0)
- OS: Ubuntu
- Pebble Version: v1.1.5
- Random Read Keys: 10M
- Warn Up Keys: 0.05% of the total keys
- Logs: [`src/bench_pebble/logs/`](https://github.com/QuarkChain/bench_kvdb/src/bench_pebble/logs/)

### Dataset Overview

| Dataset                            | Small            | Medium         | Large           |
|------------------------------------|------------------|----------------|-----------------|
| Keys                               | 200M Keys        | 2B Keys        | 20B Keys        |
| DB Size                            | 22 GB            | 224 GB         | 2.2 TB          |
| File Count                         | 1418             | 7105           | 34647           |
| Filter (without LLast) + Top Index | 32 MB (0.14%)    | 284 MB (0.12%) | 2.52 GB (0.11%) |
| Filter (with LLast)                | 238 MB           | 2.3 GB         | 23 GB           |
| All Index                          | 176 MB           | 1.7 GB         | 18 GB           |
| Filter (without LLast) + All Index | 207 MB (0.91%)   | 2.0 GB (0.89%) | 20.5 GB (0.91%) |

**Note:**

The actual percentage of these components depends on the database layout and compaction state.
Therefore, the conclusions below refer to component combinations rather than a fixed % of DB size.

#### Read I/O Cost per Get

| Cache Configuration                    | Small | Medium | Large |
|----------------------------------------|-------|--------|-------|
| **Filter (without LLast) + Top Index** | 2.25  | 2.18   | 2.42  |
| **0.2% DB Size**                       | 1.97  | 1.95   | 1.96  |
| **Filter (without LLast) + All Index** | 1.04  | 1.10   | 1.33  |
| **1% DB Size**                         | 1.03  | 1.07   | 1.31  |

![trend-io-per-get.png](images/trend-io-per-get.png)
#### Filter Hit Rate
| Dataset                                | Small  | Medium | Large |
|----------------------------------------|--------|--------| ----- |
| **Filter (without LLast) + Top Index** | 98.5%  | 99.6%  | 98.9% |
| **0.2% DB Size**                       | 100%   | 100%   | 100%  |
| **Filter (without LLast) + All Index** | 100%   | 100%   | 100%  |
| **1% DB Size**                         | 100%   | 100%   | 100%  |

#### Top Index Hit Rate
| Dataset                                | Small | Medium | Large |
|----------------------------------------|-------|--------|-------|
| **Filter (without LLast) + Top Index** | 96.4% | 97.8%  | 95.4% |
| **0.2% DB Size**                       | 100%  | 100%   | 100%  |
| **Filter (without LLast) + All Index** | 100%  | 100%   | 100%  |
| **1% DB Size**                         | 100%  | 100%   | 100%  |

#### Index Block Hit Rate
| Dataset                                | Small | Medium | Large |
|----------------------------------------| ----- |--------| ----- |
| **Filter (without LLast) + Top Index** | 2.1%  | 1.5%   | 2.9%  |
| **0.2% DB Size**                       | 9.1%  | 11.8%  | 13.7% |
| **Filter (without LLast) + All Index** | 98.2% | 93.1%  | 72.6% |
| **1% DB Size**                         | 99.6% | 95.8%  | 73.4% |

![trend-index-hit-rate.png](images/trend-index-hit-rate.png)

#### Data Block Hit Rate
| Dataset                                | Small | Medium | Large |
|----------------------------------------|-------|--------|-------|
| **Filter (without LLast) + Top Index** | 1.0%  | 0.7%   | 1.3%  |
| **0.2% DB Size**                       | 1.2%  | 0.9%   | 1.6%  |
| **Filter (without LLast) + All Index** | 1.4%  | 1.1%   | 2.4%  |
| **1% DB Size**                         | 1.5%  | 1.2%   | 2.4%  |

#### Overall Block Cache Hit Rate
| Dataset                                | Small | Medium | Large |
|----------------------------------------|-------|--------| ----- |
| **Filter (without LLast) + Top Index** | 77.3% | 79.6%  | 82.5% |
| **0.2% DB Size**                       | 80.1% | 81.7%  | 85.8% |
| **Filter (without LLast) + All Index** | 89.5% | 89.7%  | 90.4% |
| **1% DB Size**                         | 89.6% | 90.0%  | 90.5% |

![trend-blockcache-hit-rate.png](images/trend-blockcache-hit-rate.png)

#### Analysis
- When cache ≥ `Filter (without LLast) + Top Index`:
  - Filter and Top Index hit rates rise rapidly with cache size and quickly converge toward 100%.
  - Almost all negative lookups are resolved entirely in memory.
  - I/O per Get drops rapidly as cache continues to increase from around ~2.2–2.4.
- When cache ≥ `Filter (without LLast) + All Index`: 
  - I/O per Get converges toward ~1.0–1.3.
  - The rate of further I/O reduction slows down significantly as cache continues to increase. 
- Data block hit rate remains low and has **limited impact on overall I/O**.
- The **dominant factor** for I/O reduction is **Bloom filter and index residency**, not data block caching.
- These behaviors are **independent of total DB size** (22GB → 2.2TB).


## Conclusion & Recommendations
### Conclusion: Pebble Achieves O(1)-Like Read I/O Under Sufficient Cache

Although the theoretical read complexity of Pebble is `O(log N)` due to its multi-level LSM structure, 
this complexity does not directly translate into real-world read I/O behavior.

Experimental results show that:
- Once `Filter (without LLast) + Top Index` is resident in cache, almost all negative lookups are resolved entirely in memory, and I/O per Get rapidly drops to ~2 or less.
- When `Filter (without LLast) + All Index` fits in cache, I/O per Get further converges toward ~1.0–1.3, after which additional cache yields only marginal I/O reduction.

These behaviors are consistent across database sizes ranging from **22GB to 2.2TB**.

> With sufficient cache residency of Bloom filters and index blocks, the practical read I/O behavior of Pebble is 
> effectively **O(1)** and consistently converges to **1–2 I/O per Get operation**.


### Cache Configuration Recommendations

1. Minimum cache for near-constant read performance  
   The cache should be large enough to hold:
   - `Filter (without LLast) + Top Index`  
     This already eliminates almost all negative lookups and reduces I/O per Get to ~2.

2. Optimal cache for near-single-I/O reads  
   The cache should be large enough to hold:
   - `Filter (without LLast) + All Index`  
     At this point, I/O per Get consistently converges to ~1.0–1.3 even at tens of billions of keys.

3. Data block caching is optional for read I/O optimization  
