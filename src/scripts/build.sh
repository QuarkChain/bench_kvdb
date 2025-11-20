cd ../bench_rocksdb
g++ -std=c++20 main.cpp -o bench_rocksdb     -I/usr/local/include -I/usr/local/openssl/include     -L/usr/local/lib -L/usr/local/openssl/lib     -Wl,-rpath,/usr/local/openssl/lib     -lrocksdb -lpthread -lz -lsnappy -lzstd -llz4 -lbz2 -lcrypto
cd ../bench_pebble
go build