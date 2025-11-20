// main.cpp
#include <rocksdb/db.h>
#include <rocksdb/options.h>
#include <rocksdb/write_batch.h>
#include <rocksdb/statistics.h>
#include <rocksdb/cache.h>
#include <rocksdb/table.h>

#include <openssl/sha.h>
#include <iostream>
#include <vector>
#include <thread>
#include <random>
#include <chrono>
#include <cstring>
#include <getopt.h>

const int valLen = 110;
const int keyLen = 32;
static std::vector<uint8_t> randBytes(valLen * keyLen);

struct Args {
    bool needInit = false;
    long long total = 2000000000LL;
    long long writeCount = 10000000LL;
    long long readCount = 10000000LL;
    int threads = 32;
    bool batchInsert = false;
    bool forceCompact = false;
    std::string dbPath = "./data/bench_rocksdb";
    int logLevel = 3;
};

void fillRandBytes() {
    std::mt19937_64 rng((unsigned)std::chrono::steady_clock::now().time_since_epoch().count());
    for (size_t i = 0; i < randBytes.size(); ++i) {
        randBytes[i] = static_cast<uint8_t>(rng() & 0xFF);
    }
}

inline void putKey(uint8_t *key, uint64_t v) {
    uint8_t buf[8];

    // Convert uint64_t to big-endian byte array
    for (int i = 0; i < 8; ++i) {
        buf[7 - i] = static_cast<uint8_t>((v >> (i * 8)) & 0xFF);
    }

    // SHA256 hash output to key[32]
    SHA256(buf, sizeof(buf), key);
}

void batchWrite(int tid, long long count, rocksdb::DB* db, const Args &args) {
    auto st = std::chrono::steady_clock::now();
    uint8_t key[keyLen];
    memset(key, 0, keyLen);
    rocksdb::WriteBatch batch;
    rocksdb::WriteOptions wopt;
    // turn off sync for speed by default; user can edit if wants durability
    wopt.sync = false;

    for (long long i = 0; i < count; ++i) {
        uint64_t idx = static_cast<uint64_t>(tid) * static_cast<uint64_t>(count) + static_cast<uint64_t>(i);
        putKey(key, idx);
        long long s = (idx % keyLen) * valLen;
        batch.Put(rocksdb::Slice(reinterpret_cast<char*>(key), keyLen),
                  rocksdb::Slice(reinterpret_cast<char*>(&randBytes[s]), valLen));

        if (i % 1000 == 0) {
            rocksdb::Status stt = db->Write(wopt, &batch);
            if (!stt.ok()) {
                std::cerr << "batch write error: " << stt.ToString() << std::endl;
            }
            batch.Clear();
        } else {
            continue;
        }
        

        if (args.logLevel >= 3 && i > 0 && i % 1000000 == 0) {
            auto ms = std::chrono::duration_cast<std::chrono::milliseconds>(
                std::chrono::steady_clock::now() - st).count();
            std::cout << "thread " << tid << " used " << ms << " ms insert " << i << "/" << count <<", hps " << (i * 1000 / ms) << std::endl;
        }
    }

    if (batch.Count() > 0) {
        rocksdb::Status stt = db->Write(wopt, &batch);
        if (!stt.ok()) std::cerr << "batch write final error: " << stt.ToString() << std::endl;
    }

    if (args.logLevel >= 3) {
        double tu = std::chrono::duration<double>(std::chrono::steady_clock::now() - st).count();
        std::cout << "thread " << tid << " batch write done " << tu << "s, " << (count / tu) << " ops/s\n";
    }
}

void seqWrite(int tid, long long count, rocksdb::DB* db, const Args &args) {
    auto st = std::chrono::steady_clock::now();
    uint8_t key[keyLen];
    memset(key, 0, keyLen);
    rocksdb::WriteOptions wopt;
    wopt.sync = false;

    for (long long i = 0; i < count; ++i) {
        uint64_t idx = static_cast<uint64_t>(tid) * static_cast<uint64_t>(count) + static_cast<uint64_t>(i);
        putKey(key, idx);
        long long s = (idx % keyLen) * valLen;
        rocksdb::Status s2 = db->Put(wopt, rocksdb::Slice(reinterpret_cast<char*>(key), keyLen),
                                    rocksdb::Slice(reinterpret_cast<char*>(&randBytes[s]), valLen));
        if (!s2.ok()) {
            std::cerr << "put error: " << s2.ToString() << std::endl;
        }

        if (args.logLevel >= 3 && i % 1000000 == 0 && i > 0) {
            auto ms = std::chrono::duration_cast<std::chrono::milliseconds>(
                std::chrono::steady_clock::now() - st).count();
            std::cout << "thread " << tid << " used " << ms << " ms insert " << i << "/" << count << ", hps " << (i * 1000 / ms) << std::endl;
        }
    }

    if (args.logLevel >= 3) {
        double tu = std::chrono::duration<double>(std::chrono::steady_clock::now() - st).count();
        std::cout << "thread " << tid << " seq write done " << tu << "s, " << (count / tu) << " ops/s\n";
    }
}

void randomWrite(int tid, long long count, long long start, long long end, rocksdb::DB* db, const Args &args) {
    auto st = std::chrono::steady_clock::now();
    uint8_t key[keyLen];
    memset(key, 0, keyLen);
    std::mt19937_64 rng(static_cast<unsigned long>(
        std::chrono::steady_clock::now().time_since_epoch().count() + tid));
    std::uniform_int_distribution<long long> dist(start, std::max(start, end - 1));

    rocksdb::WriteOptions wopt;
    wopt.sync = false;

    for (long long i = 0; i < count; ++i) {
        uint64_t rv = static_cast<uint64_t>(dist(rng));
        putKey(key, rv);
        long long s = (rv % keyLen) * valLen;
        rocksdb::Status stt = db->Put(wopt, rocksdb::Slice(reinterpret_cast<char*>(key), keyLen),
                                      rocksdb::Slice(reinterpret_cast<char*>(&randBytes[s]), valLen));
        if (!stt.ok()) {
            std::cerr << "random put err: " << stt.ToString() << std::endl;
        }

        if (args.logLevel >= 3 && i % 1000000 == 0 && i > 0) {
            auto ms = std::chrono::duration_cast<std::chrono::milliseconds>(
                std::chrono::steady_clock::now() - st).count();
            std::cout << "thread " << tid << " used " << ms << " ms insert " << i << "/" << count << ", hps " << (i * 1000 / ms) << std::endl;
        }
    }

    if (args.logLevel >= 3) {
        double tu = std::chrono::duration<double>(std::chrono::steady_clock::now() - st).count();
        std::cout << "thread " << tid << " random write done " << tu << "s, " << (count / tu) << " ops/s\n";
    }
}

void randomRead(int tid, long long count, long long start, long long end, rocksdb::DB* db, const Args &args) {
    auto st = std::chrono::steady_clock::now();
    uint8_t key[keyLen];
    memset(key, 0, keyLen);
    std::string value;
    std::mt19937_64 rng(static_cast<unsigned long>(
        std::chrono::steady_clock::now().time_since_epoch().count() + tid));
    std::uniform_int_distribution<long long> dist(start, std::max(start, end - 1));

    rocksdb::ReadOptions ropt;
    ropt.verify_checksums = true;
    ropt.fill_cache = false;
    ropt.async_io = true;

    for (long long i = 0; i < count; ++i) {
        uint64_t rv = static_cast<uint64_t>(dist(rng));
        putKey(key, rv);
        value.clear();
        rocksdb::Status stt = db->Get(ropt, rocksdb::Slice(reinterpret_cast<char*>(key), keyLen), &value);
        if (!stt.ok() && !stt.IsNotFound()) {
            std::cerr << "random read err: " << stt.ToString() << std::endl;
        }

        if (args.logLevel >= 3 && i % 1000000 == 0 && i > 0) {
            auto ms = std::chrono::duration_cast<std::chrono::milliseconds>(
                std::chrono::steady_clock::now() - st).count();
            std::cout << "thread " << tid << " used " << ms << " ms read " << i << "/" << count << ", hps " << (i * 1000 / ms) << std::endl;
        }
    }

    if (args.logLevel >= 3) {
        double tu = std::chrono::duration<double>(std::chrono::steady_clock::now() - st).count();
        std::cout << "thread " << tid << " random read done " << tu << "s, " << (count / tu) << " ops/s\n";
    }
}

int main(int argc, char** argv) {
    Args args;

    int opt;
    // simple getopt parsing, keep param names similar to Go flags
    while ((opt = getopt(argc, argv, "n:b:c:T:t:w:r:p:l:")) != -1) {
        switch (opt) {
            case 'n': args.needInit = true; break;
            case 'b': args.batchInsert = true; break;
            case 'c': args.forceCompact = true; break;
            case 'T': args.total = std::stoll(optarg); break;
            case 't': args.threads = std::stoll(optarg); break;
            case 'w': args.writeCount = std::stoll(optarg); break;
            case 'r': args.readCount = std::stoll(optarg); break;
            case 'p': args.dbPath = std::string(optarg); break;
            case 'l': args.logLevel = std::stoi(optarg); break;
            default: break;
        }
    }

    fillRandBytes();

    rocksdb::Options options;
    options.create_if_missing = true;
    options.IncreaseParallelism(); // use background threads
    options.compression = rocksdb::kNoCompression;
    options.compaction_style = rocksdb::kCompactionStyleLevel; // optional, depends on use-case
    options.max_open_files = 100000;
    options.write_buffer_size = 128 << 20; // 64MB
    options.max_write_buffer_number = 6;
    options.target_file_size_base = 32 << 20; // 32MB
    options.max_bytes_for_level_base = 256 << 20; // L1 total size 256MB
    options.max_file_opening_threads = -1;
    options.statistics = rocksdb::CreateDBStatistics();
    options.stats_dump_period_sec = 0;

    // block cache
    rocksdb::BlockBasedTableOptions table_options;
    table_options.block_cache = rocksdb::NewLRUCache(256 << 20); // 128MB cache
    options.table_factory.reset(rocksdb::NewBlockBasedTableFactory(table_options));

    rocksdb::DB* db = nullptr;
    rocksdb::Status status = rocksdb::DB::Open(options, args.dbPath, &db);
    if (!status.ok()) {
        std::cerr << "open db error: " << status.ToString() << std::endl;
        return 1;
    }

    std::cout << "Threads: " << args.threads << std::endl;
    std::cout << "Total data: " << args.total << " while needInit=" << args.needInit << " and batchInsert=" << args.batchInsert << " and forceCompact=" << args.forceCompact << std::endl;
    std::cout << "Ops: " << args.writeCount << " write ops and " << args.readCount << " read ops" << std::endl;

    // Init writes
    if (args.needInit && args.total > 0) {
        auto start = std::chrono::steady_clock::now();
        long long per = args.total / args.threads;
        std::vector<std::thread> workers;
        for (int tid = 0; tid < args.threads; ++tid) {
            if (args.batchInsert) {
                workers.emplace_back(batchWrite, tid, per, db, std::cref(args));
            } else {
                workers.emplace_back(seqWrite, tid, per, db, std::cref(args));
            }
        }
        for (auto &t : workers) t.join();
        double ms = std::chrono::duration<double, std::milli>(std::chrono::steady_clock::now() - start).count();
        std::cout << "Init write: " << args.total << " ops in " << ms << " ms (" << (args.total * 1000.0 / ms) << " ops/s)\n";
    }

    if (args.forceCompact) {
        std::cout << "Running manual compaction..." << std::endl;
        db->CompactRange(rocksdb::CompactRangeOptions(), nullptr, nullptr);
        std::cout << "Compaction completed." << std::endl;
    }
    

    // Random writes
    if (args.writeCount > 0) {
        auto start = std::chrono::steady_clock::now();
        long long per = args.writeCount / args.threads;
        std::vector<std::thread> workers;
        for (int tid = 0; tid < args.threads; ++tid) {
            workers.emplace_back(randomWrite, tid, per, 0, args.total, db, std::cref(args));
        }
        for (auto &t : workers) t.join();
        double ms = std::chrono::duration<double, std::milli>(std::chrono::steady_clock::now() - start).count();
        std::cout << "Random update: " << args.writeCount << " ops in " << ms << " ms (" << (args.writeCount * 1000.0 / ms) << " ops/s)\n";
    }

    // Random reads
    if (args.readCount > 0) {
        auto start = std::chrono::steady_clock::now();
        long long per = args.readCount / args.threads;

        std::vector<std::thread> workers;
        for (int tid = 0; tid < args.threads; ++tid) {
            workers.emplace_back(randomRead, tid, per, 0, args.total, db, std::cref(args));
        }
        for (auto &t : workers) t.join();
        double ms = std::chrono::duration<double, std::milli>(std::chrono::steady_clock::now() - start).count();
        std::cout << "Random read: " << args.readCount << " ops in " << ms << " ms (" << (args.readCount * 1000.0 / ms) << " ops/s)\n";
    }

    auto st = options.statistics;
    std::cout << "BlockCacheHit: "
          << st->getTickerCount(rocksdb::BLOCK_CACHE_HIT) << "\n";
    std::cout << "BlockCacheMiss: "
          << st->getTickerCount(rocksdb::BLOCK_CACHE_MISS) << "\n";
    std::cout << "BytesRead (MB): "
          << st->getTickerCount(rocksdb::BYTES_READ) / (1024.0 * 1024.0) << " MB\n";

    uint64_t get_hits = st->getTickerCount(rocksdb::GET_HIT_L0)
                  + st->getTickerCount(rocksdb::GET_HIT_L1)
                  + st->getTickerCount(rocksdb::GET_HIT_L2_AND_UP);
    std::cout << "GetHits: " << get_hits << "\n";
    std::cout << "  L0 GetHits: " << st->getTickerCount(rocksdb::GET_HIT_L0) << "\n";
    std::cout << "  L1 GetHits: " << st->getTickerCount(rocksdb::GET_HIT_L1) << "\n";
    std::cout << "  L2 ~ GetHits: " << st->getTickerCount(rocksdb::GET_HIT_L2_AND_UP) << "\n";
    std::cout << "-------------------------------------\n";

    std::string stats;
    if (db->GetProperty("rocksdb.stats", &stats)) {
        std::cout << stats << "\n";
    }

    delete db;
    return 0;
}
