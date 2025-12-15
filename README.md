# bench_kvdb

`bench_kvdb` is a benchmarking tool for measuring the **practical disk I/O cost of random key-value (KV) lookups**
in **LSM-tree based databases**, using **Pebble** as the reference engine.

This project is motivated by blockchain execution workloads, where:
- State sizes reach **billions of keys**
- Reads are **highly random**
- Cache behavior dominates real performance

The benchmark focuses on a single metric:

> **I/Os per Get** — how many physical disk reads are incurred by one random KV lookup in steady state.

---

## Why This Matters

KV lookups in blockchain systems are often modeled as costing `O(log N)` disk I/O.
However, modern LSM engines rely heavily on:
- Bloom filters
- Compact index structures
- Block caches

As a result, **real disk I/O behavior can be very different from the theoretical worst case**.

This repository provides **empirical data** to measure the *actual* read I/O cost under realistic cache configurations.

---

## Key Findings (TL;DR)

Across databases from **22 GB to 2.2 TB (200M–20B keys)**:

- When the cache can hold **Bloom filters (excluding LLast) + Top Index**  
  → **I/Os per Get ≈ 2**

- When the cache can hold **Bloom filters + all index blocks**  
  → **I/Os per Get ≈ 1.0–1.3**

- Behavior is **largely independent of total DB size**
- Data block caching has **minimal impact** under pure random reads

> **Conclusion:**  
> Under sufficient cache, Pebble exhibits **effectively O(1) disk I/O** behavior for random KV lookups.

---

## Paper & Detailed Analysis

All design rationale, theory, experimental methodology, and full results are documented in:

📄 **Paper:**  
👉 [`docs/paper.md`](docs/paper.md)

The paper covers:
- Why `O(log N)` disk I/O does not reflect real LSM behavior
- Pebble’s read path and the real sources of lookup I/O
- How Bloom filters and index caching eliminate most disk reads
- Two cache inflection points that govern I/O behavior
- Empirical results on Pebble across 22 GB – 2.2 TB datasets
- Practical cache recommendations for blockchain storage systems

---

## Build & Run
This benchmark requires a small instrumentation patch to **Pebble v1.1.5**
to expose **per-call-site block cache hit statistics**.

### Patch Pebble
Replace the `readBlock` implementation in: [pebble/sstable/reader.go](https://github.com/cockroachdb/pebble/blob/v1.1.5/sstable/reader.go#L519)
with the instrumented code provided in: [src/bench_pebble/utils.go](src/bench_pebble/utils.go#L13)

The patch adds:
- Per-call-site cache **call counts**
- Per-call-site cache **hit counts**

by tracking `BlockCache.Get()` behavior inside `readBlock`.

This instrumentation is used to:
- report cache hit rates for Bloom filters, Top-Index blocks, index blocks, and data blocks;
- show how much each component contributes to `BlockCacheMiss`.

> ⚠️ This patch is for **measurement only** and is not intended for production use.


### Build

After applying the patch, build the benchmark as usual:

```bash
git clone https://github.com/QuarkChain/bench_kvdb
cd bench_kvdb/src/bench_pebble
go mod tidy
# add instrumentation
go build
```

---

### Run

**Key parameters**
- --i: initialize DB (default: false)
- --b: batch insert (default: true)
- --c: cache size (MB)
- --T: total number of keys
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

### Benchmark Environment
- CPU: 32 cores
- emory: 128 GB
- Disk: 7 TB NVMe (RAID 0)
- OS: Ubuntu
- Storage Engine: **Pebble v1.1.5**

> ⚠️ Results apply to Pebble v1.1.5.
> Read-path or cache behavior may differ in Pebble v2+.

### Results & Logs

Benchmark results and raw logs are available at:
[`src/bench_pebble/logs/`](https://github.com/QuarkChain/bench_kvdb/tree/main/src/bench_pebble/logs/)

This directory includes:
- Database properties for all datasets (Small: 22 GB; Medium: 224 GB; Large: 2.2 TB)
- Raw benchmark logs with block-cache hit/miss statistics and component breakdowns

All figures and tables in `docs/paper.md` are derived directly from these logs.

---

## Limitations
- Pure random reads only
- No range scans
- No heavy concurrent writes or compactions
- Single-node setup
- OS page cache effects not isolated

Results represent **steady-state random-read behavior**.

---

## Summary & Recommendations

`bench_kvdb` measures the **practical disk I/O cost of random KV lookups**
in LSM-based databases under blockchain-scale workloads.

If you are designing or modeling blockchain storage systems,
**do not assume `O(log N)` disk I/O — measure it.**

### Cache Recommendations

- **Minimum cache for stable reads**  
  Cache **Bloom filters + Top-Index blocks** → ~2 I/Os per Get.

- **Optimal cache for near-minimal I/O**  
  Cache **Bloom filters + all index blocks** → ~1.0–1.3 I/Os per Get.

- **Data block caching is optional** for random-read workloads.

---
