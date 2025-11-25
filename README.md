# bench_kvdb

A benchmarking tool designed to evaluate random read performance of traditional KV databases such as **RocksDB** and **PebbleDB**, specifically focusing on **IO operations per key read (IO per GET)**.

This project originated from research around the new trie database design proposed in the Base `triedb` repo:

> Traditional Key/Value stores such as LevelDB / pebble (used by geth) and MDBX (used by Reth), while extremely optimized for general-purpose arbitrary key-value workloads, are **not optimal** for persisting highly structured data like the Ethereum State Trie.  
>  
> Traversing a state trie requires `O(log N)` lookup per node, but storing nodes in a generic KV store leads to a compounded cost of `O(log N * log N)` because the database must also perform an indexed lookup for every node.

Full reference: [`base/triedb`](https://github.com/base/triedb/)

---

## Why This Project Exists

The assumption in many blockchain discussions is that RocksDB/Pebble require approximately **log(N)** lookup cost for each GET.  
Multiplied by a trie traversal `log(N)` path, the **estimated cost becomes**:


However — this assumption may be **incorrect or outdated**.

### Hypothesis Tested by This Repo

> KV storage engines like RocksDB & Pebble **should not require full logarithmic disk I/O per GET**, because:
>
> - **Block cache** reduces KV index lookups.
> - **Bloom filters** avoid unnecessary disk reads.
> - **Index and table metadata are memory resident**.
> - Modern KV implementations may achieve **~1–5 random I/O per GET**, not `log(N)`.

This repository exists to **empirically measure** that.

---

## Build & Run

### How to Build

```bash
git clone https://github.com/QuarkChain/bench_kvdb
cd bench_kvdb/scripts
./build.sh
```

### How to Run

See the README.md under `bench_pebble` and `bench_rocksdb` folder.

## Benchmark Results

### RocksDB — IO per Random Read

Random-read benchmark using **10M random keys**:
```
 Data Count    |   Size(MB)   |   IO per Key 
---------------+--------------+---------------
   200M Keys   |    28.25     |   19 ~ 21
   2B Keys     |   281.85 GB  |    2.6
   20B Keys    |    2.74 TB   |    4.0
```
The high IO count for the 200M dataset is caused by data still residing in L0 (not compacted).

After full compaction (CompactRange):
```
 Data Count    |   Size(MB)   |  IO per Key 
---------------+--------------+--------------
   200M Keys   |    28.22     |     1
   2B Keys     |   281.77 GB  |     1
   20B Keys    |    2.74 TB   |     1
```
Logs: [`src/bench_rocksdb/runlog/`](https://github.com/QuarkChain/bench_kvdb/src/bench_rocksdb/runlog/)

### PebbleDB — IO per Random Read
Random-read benchmark using 10M random keys:
```
 Data Count    |  Size(MB)  |  IO per Key 
---------------+------------+--------------
   200M Keys   |   22 GB    |    1.01
   2B Keys     |  226 GB    |    1.92
   20B Keys    |  2.2 TB    |    2.5   
```
After full compaction (Compact()):
```
 Data Count    |   Size(MB)  |  IO per Key 
---------------+-------------+--------------
   200M Keys   |    22 GB    |     1
   2B Keys     |   218 GB    |     1.72
   20B Keys    |   2.2 TB    |     1
```
Logs: [`src/bench_pebble/runlog/`](https://github.com/QuarkChain/bench_kvdb/src/bench_pebble/runlog/)
