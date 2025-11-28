echo 3 | sudo tee /proc/sys/vm/drop_caches
./bench_pebble --i --T 200000000 --w 0 --r 0 --l 2 --p ./data/bench_pebble_200m > logs/Write_200M.log
sleep 10
./bench_pebble --T 200000000 --w 0 --l 2 --t 64 --c 512 --p ./data/bench_pebble_200m
sleep 10
echo 3 | sudo tee /proc/sys/vm/drop_caches
./bench_pebble --i --T 2000000000 --w 0 --r 0 --l 2 --p ./data/bench_pebble_2b > logs/Write_2B.log
sleep 10
./bench_pebble --T 2000000000 --w 0 --l 2 --t 64 --c 512 --p ./data/bench_pebble_2b
sleep 10
echo 3 | sudo tee /proc/sys/vm/drop_caches
./bench_pebble --i --T 20000000000 --w 0 --r 0 --l 2 --p ./data/bench_pebble_20b > logs/Write_20B.log
sleep 10
./bench_pebble --T 20000000000 --w 0 --l 2 --t 64 --c 512 --p ./data/bench_pebble_20b
