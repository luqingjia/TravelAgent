# syntax=docker/dockerfile:1
# 上面这行告诉 Docker 使用 BuildKit 的 Dockerfile 语法版本 1。
# 好处：构建缓存、多阶段构建等现代特性更稳定；没有它也能构建，但建议保留。

# =============================================================================
# TravelAgent 镜像构建说明（大白话）
# -----------------------------------------------------------------------------
# 这个 Dockerfile 分两段：
# 1) builder 阶段：用带 Go 工具链的镜像把源码编译成一个可执行文件
# 2) runtime 阶段：用很小的 Alpine 镜像，只放二进制和运行必需依赖
#
# 为什么要两段？
# - 编译需要完整 Go 环境，镜像很大
# - 真正跑服务时不需要源码和编译器
# - 最终镜像更小、攻击面更小，也更适合部署
# =============================================================================

# ---------- 第一阶段：编译 ----------
# 使用官方 Go 1.26 + Debian bookworm 作为编译环境。
# 镜像标签必须和 go.mod 的 go 版本大致匹配，否则可能出现语法/标准库不兼容。
FROM golang:1.26-bookworm AS builder

# 把构建容器内的工作目录切到 /src。
# 后面的 COPY、RUN 如果写相对路径，都相对这个目录。
WORKDIR /src

# 先只复制依赖清单 go.mod / go.sum。
# 这样做是为了利用 Docker 分层缓存：
# - 只要依赖没变，下一行 go mod download 可以直接复用缓存
# - 源码天天改也不会每次都重新下载依赖
COPY go.mod go.sum ./

# 下载 Go 模块依赖到模块缓存。
# 失败通常是网络问题或 go.sum 与 go.mod 不一致。
RUN go mod download

# 再复制整个构建上下文中的源码。
# 注意：真正复制哪些文件，还受仓库根目录 .dockerignore 控制。
# 测试、文档、.git、本地密钥等会被忽略，缩短构建上下文并避免泄密。
COPY . .

# 编译出 Linux 静态（或尽量静态）二进制。
# 逐项解释：
# - CGO_ENABLED=0：关掉 cgo，避免依赖宿主机 C 库，方便放进 Alpine
# - GOOS=linux：目标操作系统是 Linux 容器
# - GOARCH=amd64：目标 CPU 架构；如果要在 ARM 机器跑，需要改成 arm64 或用 buildx
# - -trimpath：去掉二进制里的本地路径信息，构建结果更可复现
# - -ldflags="-s -w"：去掉符号表和 DWARF 调试信息，缩小体积
# - -o /out/travel-agent：输出文件路径
# - ./cmd/travel-agent：程序入口包
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" \
    -o /out/travel-agent ./cmd/travel-agent

# ---------- 第二阶段：运行 ----------
# Alpine 很小，适合放已经编译好的二进制。
# 版本钉死 3.21，避免 latest 突然变导致构建不可复现。
FROM alpine:3.21

# 安装运行期系统依赖，并创建非 root 用户。
# - ca-certificates：访问 HTTPS Embedding API 时需要系统 CA
# - tzdata：容器内时间/时区数据；日志时间才和 Asia/Shanghai 对齐
# - wget：给 compose healthcheck 探测 /health 用
# - adduser -D -H -u 10001 travelagent：
#   -D 不设密码，-H 不建 home 目录，固定 uid 方便卷权限管理
RUN apk add --no-cache ca-certificates tzdata wget \
    && adduser -D -H -u 10001 travelagent

# 运行阶段工作目录。当前程序其实不依赖工作目录里的文件，
# 但保留 /app 作为常规约定，便于以后挂配置或调试。
WORKDIR /app

# 只从 builder 阶段拷贝编译好的二进制，不带源码和 Go 工具链。
COPY --from=builder /out/travel-agent /usr/local/bin/travel-agent

# 预先创建本地对象存储目录，并把属主改成非 root 用户。
# 原因：
# 1) 程序默认可能写 LOCAL_STORAGE_DIR=/data/storage
# 2) 容器以 travelagent 用户运行，若目录属于 root，会写入失败
# compose 会再把命名卷挂到 /data/storage，数据可持久化
RUN mkdir -p /data/storage && chown -R travelagent:travelagent /data

# 切换到非 root 用户启动进程。
# 这样即使应用被攻破，也不直接拥有容器 root 权限。
USER travelagent

# 声明容器内监听端口（文档性质）。
# 真正映射到宿主机端口是在 docker-compose.yml 的 ports 里做的。
EXPOSE 8081

# 容器启动时直接运行 travel-agent。
# 重要：Go 程序不会自动读取 .env 文件；
# 所有配置必须由 compose 的 environment / --env-file 注入到进程环境变量。
ENTRYPOINT ["/usr/local/bin/travel-agent"]
