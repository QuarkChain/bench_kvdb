FROM ubuntu:24.04

# 避免交互
ENV DEBIAN_FRONTEND=noninteractive

# 安装依赖
RUN apt-get update && apt-get upgrade -y && apt-get install -y \
    build-essential \
    git \
    cmake \
    libgflags-dev \
    libsnappy-dev \
    zlib1g-dev \
    libbz2-dev \
    liblz4-dev \
    libzstd-dev \
    libssl-dev \
    vim \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# 设置 RocksDB 版本号（可替换为最新标签）
ARG ROCKSDB_VERSION=v10.7.5
RUN cd /usr/src \
    && git clone --depth 1 --branch ${ROCKSDB_VERSION} https://github.com/facebook/rocksdb.git \
    && cd rocksdb \
    && DEBUG_LEVEL=0 make shared_lib \
    && make install-shared \
    && ldconfig

# 创建工作目录
WORKDIR /root

#（可选）复制你的 benchmark 脚本到容器内
COPY . ./bench


# 默认启动脚本
CMD [ "/bin/bash" ]

