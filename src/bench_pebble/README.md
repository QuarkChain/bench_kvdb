## Run commands: 
### Build
```bash
go mod tidy 
go build 
```

**Usage：**
  - --ni：init insert data 
  - --bi: batch insert
  - --fc: force compact after init insert data
  - --T：total number of keys count
  - --t: threads
  - --w：random write count 
  - --r：random read count
  - --p：db path
  - --l：log level

### Sample run 200M keys with force compact
```bash
mkdir -p ./data
./bench_pebble --ni --T 200000000 --w 0 --r 0 --l 2 --fc > runlog/Write_200M_C.log
sleep 10
./bench_pebble --T 200000000 --w 0 --l 2 --t 64 > runlog/RadmonRead_200M_C_1_Hot.log
sleep 10
echo 3 | sudo tee /proc/sys/vm/drop_caches
./bench_pebble --T 200000000 --w 0 --l 2 --t 64 > runlog/RadmonRead_200M_C_2_Cold.log
sleep 10
./bench_pebble --T 200000000 --w 0 --l 2 --t 64 > runlog/RadmonRead_200M_C_3_hot.log
sleep 10
./bench_pebble --T 200000000 --w 0 --l 2 --t 64 > runlog/RadmonRead_200M_C_4_hot.log
sleep 10
./bench_pebble --T 200000000 --w 0 --l 2 --t 64 > runlog/RadmonRead_200M_C_5_hot.log
sleep 10
echo 3 | sudo tee /proc/sys/vm/drop_caches
./bench_pebble --T 200000000 --w 0 --l 2 --t 64 > runlog/RadmonRead_200M_C_6_Cold.log
sleep 10
./bench_pebble --T 200000000 --w 0 --l 2 --t 64 > runlog/RadmonRead_200M_C_7_hot.log
```

### Sample run 2B keys without force compact
```bash
mkdir -p ./data
./bench_pebble --ni --T 2000000000 --w 0 --r 0 --l 2 > runlog/Write_2B.log
sleep 10
./bench_pebble --T 2000000000 --w 0 --l 2 --t 64 > runlog/RadmonRead_2B_1_Hot.log
sleep 10
echo 3 | sudo tee /proc/sys/vm/drop_caches
./bench_pebble --T 2000000000 --w 0 --l 2 --t 64 > runlog/RadmonRead_2B_2_Cold.log
sleep 10
./bench_pebble --T 2000000000 --w 0 --l 2 --t 64 > runlog/RadmonRead_2B_3_hot.log
sleep 10
./bench_pebble --T 2000000000 --w 0 --l 2 --t 64 > runlog/RadmonRead_2B_4_hot.log
sleep 10
./bench_pebble --T 2000000000 --w 0 --l 2 --t 64 > runlog/RadmonRead_2B_5_hot.log
sleep 10
echo 3 | sudo tee /proc/sys/vm/drop_caches
./bench_pebble --T 2000000000 --w 0 --l 2 --t 64 > runlog/RadmonRead_2B_6_Cold.log
sleep 10
./bench_pebble --T 2000000000 --w 0 --l 2 --t 64 > runlog/RadmonRead_2B_7_hot.log
```