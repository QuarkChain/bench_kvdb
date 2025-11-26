## Run commands: 
```bash
sudo apt update
sudo apt install -y build-essential cmake libgflags-dev libsnappy-dev zlib1g-dev libbz2-dev liblz4-dev libzstd-dev
sudo apt install -y librocksdb-dev

mkdir build && cd build
cmake ..
make -j$(nproc)

cd ..
g++ -std=c++20 main.cpp -o bench_rocksdb     -I/usr/local/include -I/usr/local/openssl/include     -L/usr/local/lib -L/usr/local/openssl/lib     -Wl,-rpath,/usr/local/openssl/lib     -lrocksdb -lpthread -lz -lsnappy -lzstd -llz4 -lbz2 -lcrypto

# create data folder
mkdir -p ./data
```

**Note: **
  - RocksDB version should be larger than 10.x.x
  - You can use Dockerfile to build a clean docker with RocksDB with 10.7.5 to run this bench
  - You can run command using `monitor.sh` like `./monitor.sh ./bench_rocksdb -T 200000000 -w 0 -l 2 -t 64` to show the io read/write from os
```
# Sample 

rchar: 49515807199
wchar: 1075344843
syscr: 12130719 
syscw: 1124
read_bytes: 237568
write_bytes: 1075433472
cancelled_write_bytes: 0
----------------------------------
```

| Field | Value | Meaning |
|-------|-------|---------|
| `rchar` | **44,390,007,322 bytes (~44.4 GB)** | Total bytes the process requested to read via system calls. Includes reads served from the page cache, not only physical disk I/O. |
| `wchar` | **1,282,566 bytes (~1.22 MB)** | Total bytes written through write system calls. Not necessarily persisted to disk (may be buffered). |
| `syscr` | **10,046,363 calls** | Number of read system calls (`read()`, `pread()`, etc.). Indicates many small reads or repeated random access patterns. |
| `syscw` | **69 calls** | Number of write system calls — extremely low, meaning writes are heavily buffered, batched, or minimal. |
| `read_bytes` | **50,904,911,872 bytes (~50.9 GB)** | Actual physical bytes read from storage hardware (not from cache). Represents real disk I/O cost. |
| `write_bytes` | **1,294,336 bytes (~1.23 MB)** | Actual bytes written to storage. Very small, implying mostly read-heavy or append-only workload. |
| `cancelled_write_bytes` | **0** | No cancelled kernel write operations. |


### Sample run
```bash
./bench_rocksdb -n -T 2000000000 -t 16 -w 0 -r 1000000 
```
**Usage：**
  - -n：init insert data 
  - -b: batch insert
  - -c: force compact after init insert data
  - -T：total number of keys count
  - -t: threads
  - -w：random write count 
  - -r：random read count
  - -p：db path
  - -l：log level


### Sample run 200M keys with force compact
```bash
mkdir -p ./data
./bench_rocksdb -l 2 -T 200000000 -n true -b true -w 0 -r 0 -c true > runlog/Write_200M_C.log
sleep 10
./bench_rocksdb -T 200000000 -w 0 -l 2 -t 64 > runlog/RadmonRead_200M_C_1_Hot.log
sleep 10
echo 3 | sudo tee /proc/sys/vm/drop_caches
./bench_rocksdb -T 200000000 -w 0 -l 2 -t 64 > runlog/RadmonRead_200M_C_2_Cold.log
sleep 10
./bench_rocksdb -T 200000000 -w 0 -l 2 -t 64 > runlog/RadmonRead_200M_C_3_hot.log
sleep 10
./bench_rocksdb -T 200000000 -w 0 -l 2 -t 64 > runlog/RadmonRead_200M_C_4_hot.log
sleep 10
./bench_rocksdb -T 200000000 -w 0 -l 2 -t 64 > runlog/RadmonRead_200M_C_5_hot.log
sleep 10
echo 3 | sudo tee /proc/sys/vm/drop_caches
./bench_rocksdb -T 200000000 -w 0 -l 2 -t 64 > runlog/RadmonRead_200M_C_6_Cold.log
sleep 10
./bench_rocksdb -T 200000000 -w 0 -l 2 -t 64 > runlog/RadmonRead_200M_C_7_hot.log
```

### Sample run 2B keys without force compact
```bash
mkdir -p ./data
./bench_rocksdb -l 2 -T 2000000000 -n true -b true -w 0 -r 0 > runlog/Write_2B.log
sleep 10
./bench_rocksdb -T 2000000000 -w 0 -l 2 -t 64 > runlog/RadmonRead_2B_1_Hot.log
sleep 10
echo 3 | sudo tee /proc/sys/vm/drop_caches
./bench_rocksdb -T 2000000000 -w 0 -l 2 -t 64 > runlog/RadmonRead_2B_2_Cold.log
sleep 10
./bench_rocksdb -T 2000000000 -w 0 -l 2 -t 64 > runlog/RadmonRead_2B_3_hot.log
sleep 10
./bench_rocksdb -T 2000000000 -w 0 -l 2 -t 64 > runlog/RadmonRead_2B_4_hot.log
sleep 10
./bench_rocksdb -T 2000000000 -w 0 -l 2 -t 64 > runlog/RadmonRead_2B_5_hot.log
sleep 10
echo 3 | sudo tee /proc/sys/vm/drop_caches
./bench_rocksdb -T 2000000000 -w 0 -l 2 -t 64 > runlog/RadmonRead_2B_6_Cold.log
sleep 10
./bench_rocksdb -T 2000000000 -w 0 -l 2 -t 64 > runlog/RadmonRead_2B_7_hot.log
```
