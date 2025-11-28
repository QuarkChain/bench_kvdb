./bench_pebble --T 200000000 --w 0 --l 2 --t 64 --c 16 --p ./data/bench_pebble_200m > logs/RadmonRead_200M_16M.log
sleep 10
./bench_pebble --T 200000000 --w 0 --l 2 --t 64 --c 512 --p ./data/bench_pebble_200m > logs/RadmonRead_200M_512M.log
sleep 10
./bench_pebble --T 200000000 --w 0 --l 2 --t 64 --c 5120 --p ./data/bench_pebble_200m > logs/RadmonRead_200M_5120M.log

sleep 10
./bench_pebble --T 2000000000 --w 0 --l 2 --t 64 --c 16 --p ./data/bench_pebble_2b > logs/RadmonRead_2B_16M.log
sleep 10
./bench_pebble --T 2000000000 --w 0 --l 2 --t 64 --c 512 --p ./data/bench_pebble_2b > logs/RadmonRead_2B_512M.log
sleep 10
./bench_pebble --T 2000000000 --w 0 --l 2 --t 64 --c 5120 --p ./data/bench_pebble_2b > logs/RadmonRead_2B_5120M.log

sleep 10
./bench_pebble --T 20000000000 --w 0 --l 2 --t 64 --c 16 --p ./data/bench_pebble_20b > logs/RadmonRead_20B_16M.log
sleep 10
./bench_pebble --T 20000000000 --w 0 --l 2 --t 64 --c 512 --p ./data/bench_pebble_20b > logs/RadmonRead_20B_512M.log
sleep 10
./bench_pebble --T 20000000000 --w 0 --l 2 --t 64 --c 51200 --p ./data/bench_pebble_20b > logs/RadmonRead_20B_51200M.log
sleep 10
./bench_pebble --T 20000000000 --w 0 --l 2 --t 64 --c 51200 --p ./data/bench_pebble_20b > logs/RadmonRead_20B_51200M.log