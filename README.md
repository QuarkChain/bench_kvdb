# bench_kvdb

## Abstract

Modern blockchain systems rely on large-scale key-value (KV) databases to serve highly random state accesses at billions
of keys. While the theoretical read complexity of LSM-based storage engines such as Pebble is often described as `O(log N)`, 
this model does not accurately reflect real disk I/O behavior under realistic cache conditions.

This work presents an extensive empirical study on the **true disk I/O cost of random KV lookups at blockchain scale**, using Pebble as the underlying storage engine. We benchmark databases ranging from **22 GB to 2.2 TB** (200M to 20B keys) under multiple cache configurations that selectively fit Bloom filters, top indexes, and full index blocks into memory.

Our experiments show that:
- Once **Bloom filters (excluding LLast) and Top-Index blocks** fit in cache, **most negative lookups incur zero disk I/O**, and the I/O per Get rapidly drops to ~2.
- When **all index blocks also fit in cache**, the I/O per Get further converges to **~1.0–1.3**, largely independent of total database size.
- Data block caching has only a secondary effect on overall I/O.

These results demonstrate that, under sufficient cache, **Pebble exhibits effectively O(1) disk I/O behavior for random reads**, 
challenging the common assumption that each KV lookup inherently costs `O(log N)` physical I/Os. This has direct implications 
for the performance modeling and design of blockchain trie databases and execution-layer storage systems.

---

## Overview

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

## Motivation

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

---

## Background
### Pebble’s Level Structure 

Pebble uses a level-based LSM structure where data flows from memory tables into SST files across levels `L0 → LBase → LLast`.
Each level has larger capacity than the previous one (typically ×10), but Pebble treats this growth ratio as a soft 
heuristic rather than a strict rule.

In practice:
- **LBase** is the level selected as the compaction target for L0. 
  It changes dynamically depending on level sizes (e.g., when L1 almost empty, L2 becomes LBase).
- **LLast (Last Level)** is the current deepest level (initially L6) and holding the majority of the data; 
  Pebble adds deeper levels (L7, L8, …) as the DB grows beyond the capacity of the existing last level.

The diagram below shows an example layout:

![ethereum.png](./images/ethereum.png)

In this snapshot:
- 7.9B key-values are stored in **14,840 SST files**.
- L2 is chosen as **LBase** because L1 is empty and L2 holds only ~54 MB.
- Most data sits in **L6 (~393 GB)**, making it the current LLast (deepest level).

---

### SST File Structure and Usage in Pebble

SST (Sorted String Table) is the core storage unit in Pebble, which contains multiple sections:

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

---

### Read Path in Pebble
A typical Get (key lookup) proceeds as follows: 
1. **MemTable / Immutable MemTables (memory only)**
    - If key exists here, return immediately.

2. **Consult MANIFEST Metadata(memory only)**
    - MANIFEST is fully in-memory and identifies which SSTs may contain the key.

3. **Search SSTs by level**
    - **L0**: Files may overlap → multiple SSTs may be checked.
    - **`LBase` and higher**: Non-overlapping → at most **one SST per level**.

   For each SST file
    - **a) Top-Index Load at Init**
        - Very small structure, **usually cached** → **~0 I/O**.

    - **b) Table-level Bloom Filter Check (except LLast)**
        - **Usually cached**; if not, 1 I/O → **~0 I/O**.
        - If filter says key definitely absent → skip SST with no further I/O.

    - **c) Index Block Lookup**
        - Performed only if the filter indicates “may exist.”
        - Cached or 1 I/O → **0–1 I/O**.

    - **d) Data Block Read**
        - Located via the index block.
        - **Typically, not cached → ~1 I/O**.

   Thanks to the Bloom Filter Check (except LLast):
      - Negative lookups → almost zero I/O.
      - Positive lookups → typically 1–2 I/Os depending on cache.


#### Special Optimization for the Last Level (LLast)

Pebble **disables Bloom filters** for LLast. Instead, lookups rely directly on the index block.

**Reason:**
- LLast contains the **majority of the DB**.
- Its Bloom filter would be:
  - Very large
  - Inefficient to cache
  - Expensive in memory
  - Provide limited benefit (most keys end up in LLast)

**As a result:**
- The block index provides efficient block-level pruning and is used to locate the data block.
- This:
  - Saves cache space
  - Avoids one extra filter read
  - Reduces memory pressure and total I/O

---

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

---

## Hypothesis and Verification
### Why `O(log N)` Does Not Reflect Reality
In real blockchain workloads (e.g., Geth, Optimism), the theoretical `O(log N)` bound significantly overestimates real 
disk I/O, mainly due to the behavior of Bloom filters and cache residency.
1. **Bloom Filters Prevent Most Useless I/O**
    - Bloom filters are checked before the index and data blocks.
    - If the filter says “key does not exist”, the SST is skipped with:
      - No index block read
      - No data block read
    - This effectively removes most negative lookups from touching disk.

2. **Bloom Filters Excluding LLast + Top-Index Are Small and Highly Reused**
    - Their combined size is typically small (e.g., `≈ 0.18%` of total DB size in Ethereum)
    - They are accessed on nearly every `Get`, have very high reuse, and are extremely cache-friendly.
    - Therefore, if:  
      **Cache Size > Bloom filters (without LLast) + Top-Index size**,
    - then
      - Almost all Bloom filters and Top-Indexes remain resident in memory
      - Bloom filter lookups → **~0 I/O**
      - Top-Index block → **0 I/O**
      - Combined with index and data block access, positive lookups generally reduce to:  
        **1–2 I/Os per Get**.

3. **I/O Cost with Sufficient Cache**
    - Cache can hold: `Bloom filters (excluding LLast)` + `All Index blocks` (`≈ 1.3%` of Ethereum DB size)
    - Then for most `Get` operations require only a single data-block read → effectively O(1) I/O.
      - Bloom filter → **0 I/O**
      - Top-Index block → **0 I/O**
      - Index block → **0 I/O**
      - Only the data block read remains → **~1 I/O**
    - So the real cost becomes:
      - `≈ 1 I/Os per Get` operation → Effectively O(1)
      - If hot data blocks also fit in cache, the amortized I/O can even fall **below 1 I/O per Get**.

**We hypothesize that**
> With sufficient cache, Pebble’s practical read I/O approaches an *O(1)* pattern, converging to ~1–2 I/Os per Get — far below the theoretical `O(log N)` complexity.

---

### Benchmark Plan: Validating the O(1)-Like Read Behavior

To verify the previous hypothesis, the benchmark will:

1. Use **Pebble** (the same storage engine used by Geth).

2. Test multiple cache configurations:
    - From 0.1% of the DB Size, which is smaller than `Filter (excluding LLast) + Top Index`
    - To 3% of the DB Size, which is larger than `Filter (excluding LLast) + All Index`

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

---

## Benchmark Results

Before diving into detailed graphs, here are the key results consistently across all sizes of datasets:
1. When cache > Filter(excluding LLast) + Top Index:
    - Bloom & Top-Index hit → ~100%
    - I/Os per Get → ~2 (O(1))

2. When cache > Filter(excluding LLast) + All Index:
    - Index hit → 70–99%
    - I/Os per Get → 1.0–1.3

### Environment
- CPU: 32 cores
- Memory: 128 GB
- Disk: 7 TB (2 * SAMSUNG MZQL23T8HCLS-00A07 + RAID 0)
- OS: Ubuntu
- Pebble Version: v1.1.5
- Random Read Keys: 10M
- Warn Up Keys: 0.05% of the total keys
- Logs: [`src/bench_pebble/logs/`](https://github.com/QuarkChain/bench_kvdb/src/bench_pebble/logs/)

### Limitations
- Measures pure random reads only
- No range scans
- No heavy write + compaction interference
- Single-node only
- OS page cache effects are not isolated
- **Pebble Version: v1.1.5** — behavior in **Pebble v2+ may differ** due to ongoing changes in compaction, block format, and caching policies.

Results represent steady-state random-read behavior under sufficient cache.

---

### Dataset Overview

| Dataset                              | Small            | Medium         | Large           |
|--------------------------------------|------------------|----------------|-----------------|
| Keys                                 | 200M Keys        | 2B Keys        | 20B Keys        |
| DB Size                              | 22 GB            | 224 GB         | 2.2 TB          |
| File Count                           | 1418             | 7105           | 34647           |
| Filter (excluding LLast) + Top Index | 32 MB (0.14%)    | 284 MB (0.12%) | 2.52 GB (0.11%) |
| Filter (including LLast)             | 238 MB           | 2.3 GB         | 23 GB           |
| All Index                            | 176 MB           | 1.7 GB         | 18 GB           |
| Filter (excluding LLast) + All Index | 207 MB (0.91%)   | 2.0 GB (0.89%) | 20.5 GB (0.91%) |

**Note:**

The actual percentage of these components depends on the database layout and compaction state.
Therefore, the conclusions below refer to item name like `Filter (excluding LLast) + Top Index` rather than a fixed % of DB size.

---

### Terminology & Definitions

The analysis below will use two **inflection points** and three **cache phases**, based on the cache size relative to Bloom filters and index blocks.

#### Two Inflection Points

- **Inflection Point 1 — `Filter (excluding LLast) + Top Index`**  
  Cache is large enough to hold:
    - All Bloom filters (excluding LLast)
    - All top-level index blocks

- **Inflection Point 2 — `Filter (excluding LLast) + All Index`**  
  Cache is large enough to hold:
    - All Bloom filters (excluding LLast)
    - All index blocks across all levels

---

#### Three Phases

- **Phase 1 — Up to `Filter (excluding LLast) + Top Index`**  
  `Cache < Inflection Point 1`

- **Phase 2 — Between the two inflection points**  
  `Inflection Point 1 < Cache < Inflection Point 2`

- **Phase 3 — Beyond `Filter (excluding LLast) + All Index`**  
  `Cache > Inflection Point 2`

---

All subsequent analyses (filter hit rates, index hit rates, data hit rates, overall block cache hit rate, and I/O per Get) directly reference these standardized definitions.


### Filter Hit Rate & Top Index Hit Rate

| Dataset                                                            | Small (Filter) | Medium (Filter) | Large (Filter) | Small (TopIdx) | Medium (TopIdx) | Large (TopIdx) |
|--------------------------------------------------------------------|----------------|-----------------|----------------|----------------|-----------------|----------------|
| **At Inflection Point 1** (`Filter (excluding LLast) + Top Index`) | 98.5%          | 99.6%           | 98.9%          | 96.4%          | 97.8%           | 95.4%          |
| **0.2% DB Size**                                                   | 100%           | 100%            | 100%           | 100%           | 100%            | 100%           |
| **At Inflection Point 2** (`Filter (excluding LLast) + All Index`) | 100%           | 100%            | 100%           | 100%           | 100%            | 100%           |
| **1% DB Size**                                                     | 100%           | 100%            | 100%           | 100%           | 100%            | 100%           |

**Analysis.**  
Once the cache exceeds Inflection Point 1, both the Bloom filter and Top Index achieve near-100% hit rate and negative lookups are resolved in memory.

---

### Index Block Hit Rate

| Dataset                   | Small | Medium | Large |
|---------------------------|-------|--------|-------|
| **At Inflection Point 1** | 2.1%  | 1.5%   | 2.9%  |
| **0.2% DB Size**          | 9.1%  | 11.8%  | 13.7% |
| **At Inflection Point 2** | 98.2% | 93.1%  | 72.6% |
| **1% DB Size**            | 99.6% | 95.8%  | 73.4% |

![trend-index-hit-rate.png](images/trend-index-hit-rate.png)

**Analysis.**  
1. **Phase 1:** Very few index blocks cached (may **~1%–3%**).
2. **Phase 2:** Index hit **rises sharply** to ~70–99% as the cache approaches Inflection Point 2.
3. **Phase 3:** Most index blocks reside in memory, and the hit rate reaches a **high plateau (~70%–99%)** with only marginal further gains.

---

### Data Block Hit Rate

| Dataset                   | Small | Medium | Large |
|---------------------------|-------|--------|-------|
| **At Inflection Point 1** | 1.0%  | 0.7%   | 1.3%  |
| **0.2% DB Size**          | 1.2%  | 0.9%   | 1.6%  |
| **At Inflection Point 2** | 1.4%  | 1.1%   | 2.4%  |
| **1% DB Size**            | 1.5%  | 1.2%   | 2.4%  |

**Analysis.**  
Across all three phases, data block hit rate remains consistently below **3%**,
**data block caching contributes little to the observed I/O reduction** in random-read workloads.

---

### Overall Block Cache Hit Rate

| Dataset                   | Small | Medium | Large |
|---------------------------|-------|--------|-------|
| **At Inflection Point 1** | 77.3% | 79.6%  | 82.5% |
| **0.2% DB Size**          | 80.1% | 81.7%  | 85.8% |
| **At Inflection Point 2** | 89.5% | 89.7%  | 90.4% |
| **1% DB Size**            | 89.6% | 90.0%  | 90.5% |

![trend-blockcache-hit-rate.png](images/trend-blockcache-hit-rate.png)

**Analysis.**
1. **Phase 1:** Hit rate rises steeply, driven by the rapid **in-memory residency of Bloom filters and Top Index**.
2. **Phase 2:** Hit rate grow at a **slower slope** driven by index blocks become resident.
3. **Phase 3:** Hit rate **stabilizes**, since data block caching contributes little under random read workloads.

---

### Read I/O Cost per Get

| Cache Configuration       | Small | Medium | Large |
|---------------------------|-------|--------|-------|
| **At Inflection Point 1** | 2.25  | 2.18   | 2.42  |
| **0.2% DB Size**          | 1.97  | 1.95   | 1.96  |
| **At Inflection Point 2** | 1.04  | 1.10   | 1.33  |
| **1% DB Size**            | 1.03  | 1.07   | 1.31  |

![trend-io-per-get.png](images/trend-io-per-get.png)

**Analysis.**  
1. **Phase 1:** **I/Os per Get** drop quickly to **effectively O(1)** (~2.2–2.4) as cache approaches Inflection Point 1.
2. **Phase 2:** **I/Os per Get** drop steeply toward ~1 (1.0–1.3) as more index blocks enter cache.
3. **Phase 3:** **I/Os per Get** reach a **near-minimal plateau ~1** and further cache expansion yields **only marginal gains**.

---

### Key Observations

- **Inflection Point 1 (`Filter (excluding LLast) + Top Index`)**  
  Once the cache reaches this point:
    - Filter & Top Index hit rates reach **~100%**.
    - Most **negative lookups are resolved entirely in memory**.
    - Random-read `I/Os per Get` **stabilize** at ~2.2–2.4 (**effectively O(1) lookups**).
- **Phase 2 (between the two Inflection Points)**  
  In this transition region:
    - Index block residency grows rapidly. 
    - Index block hit rate rises sharply to **~70%–99%**.
    - **I/Os per Get drop sharply toward 1.0–1.3.**
- **Inflection Point 2 (`Filter (excluding LLast) + All Index`)**  
  Beyond this point:
    - Random-read **`I/Os per Get` approach the tight lower bound** (~1 `I/O per Get`).
    - Further cache growth yields **only marginal additional I/O reduction**.
- **Data block caching remains negligible in all phases.**
- **Behavior consistent across dataset sizes (22 GB – 2.2 TB):**  

> Overall, random-read I/O is primarily governed by Bloom filter and index residency.

---

## Conclusion & Recommendations
### Conclusion: Pebble Achieves O(1)-Like Read I/O Under Sufficient Cache

Although the theoretical read complexity of Pebble is `O(log N)` due to its multi-level LSM structure, 
this complexity does not directly translate into real-world read I/O behavior.

Experimental results show that:
- Once `Filter (excluding LLast) + Top Index` is resident in cache, almost all negative lookups are resolved entirely in memory, and I/O per Get rapidly drops to ~2 or less.
- When `Filter (excluding LLast) + All Index` fits in cache, I/O per Get further converges toward ~1.0–1.3, after which additional cache yields only marginal I/O reduction.

These behaviors are consistent across database sizes ranging from **22GB to 2.2TB**.

> With sufficient cache residency of Bloom filters and index blocks, the practical read I/O behavior of Pebble is 
> effectively **O(1)** and consistently converges to **1–2 I/O per Get operation**.

---

### Cache Configuration Recommendations

1. Minimum cache for near-constant read performance  
   The cache should be large enough to hold:
   - `Filter (excluding LLast) + Top Index`  
     This already eliminates almost all negative lookups and reduces I/O per Get to ~2.

2. Optimal cache for near-single-I/O reads  
   The cache should be large enough to hold:
   - `Filter (excluding LLast) + All Index`  
     At this point, I/O per Get consistently converges to ~1.0–1.3 even at tens of billions of keys.

3. Data block caching is optional for read I/O optimization  

---