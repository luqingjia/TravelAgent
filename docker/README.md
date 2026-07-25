# Docker 部署说明（含配置解释）

这个目录提供 TravelAgent 的 Docker 构建与 Compose 编排，方便你一条命令起本地/服务器环境。

## 这套东西会启动什么

默认（不加 profile）：

1. `postgres`：PostgreSQL 16 + pgvector
2. `app`：TravelAgent HTTP 服务（默认本地文件存储）

可选（加 `--profile s3`）：

3. `minio`：S3 兼容对象存储
4. `minio-init`：第一次自动创建 bucket

## 重要前提（先看）

1. **Go 程序不会自动读 `.env` 文件**  
   它只认进程环境变量。`docker compose --env-file docker/.env` 负责把变量注入容器。

2. **数据库迁移不会在应用启动时执行**  
   只有 PostgreSQL 数据卷是空的、第一次初始化时，才会执行：
   - `migrations/000001_rag_baseline.sql`
   - `docker/initdb/02-seed-demo-kb.sql`

3. **`000001_rag_baseline.sql` 含 `DROP TABLE`**  
   只适合空库。不要对已有业务数据重复执行。

4. **上传前需要知识库已存在**  
   演示库 ID 固定为 `kb_demo_001`。

## 1. 准备环境变量

```bash
cp docker/env.example docker/.env
```

编辑 `docker/.env`：

- 必改：`EMBEDDING_API_KEY=你的真实密钥`
- 可选：端口、数据库密码、对象存储模式

每个变量的逐行解释见 `docker/env.example` 里的中文注释。

## 2. 启动（本地存储，推荐）

在仓库根目录执行：

```bash
docker compose -f docker/docker-compose.yml --env-file docker/.env up -d --build
```

检查是否正常：

```bash
docker compose -f docker/docker-compose.yml --env-file docker/.env ps
curl http://localhost:8081/health
```

## 3. 启动（MinIO / S3 兼容存储）

1. 修改 `docker/.env`：

```env
RUSTFS_ENABLED=true
RUSTFS_BUCKET_NAME=travelagent
RUSTFS_ENDPOINT=http://minio:9000
RUSTFS_ACCESS_KEY=minioadmin
RUSTFS_SECRET_KEY=minioadmin
RUSTFS_PATH_STYLE=true
```

2. 带 profile 启动：

```bash
docker compose -f docker/docker-compose.yml --env-file docker/.env --profile s3 up -d --build
```

MinIO 控制台默认：`http://localhost:9001`  
账号密码默认就是上面的 `minioadmin / minioadmin`。

## 4. 演示知识库与上传

| 字段 | 值 |
|---|---|
| 知识库 ID | `kb_demo_001` |
| 名称 | Docker Demo Knowledge Base |

上传示例：

```bash
curl -X POST "http://localhost:8081/api/knowledge/bases/kb_demo_001/documents/upload" \
  -F "file=@./README.md"
```

## 5. 常用命令

```bash
# 看应用日志
docker compose -f docker/docker-compose.yml --env-file docker/.env logs -f app

# 停止容器（保留数据卷）
docker compose -f docker/docker-compose.yml --env-file docker/.env down

# 停止并删除数据卷（数据库和本地上传文件都会清空）
docker compose -f docker/docker-compose.yml --env-file docker/.env down -v

# 只重建应用镜像并重启 app
docker compose -f docker/docker-compose.yml --env-file docker/.env up -d --build app
```

## 6. 目录里每个文件干什么

| 路径 | 作用 |
|---|---|
| `Dockerfile` | 多阶段构建 travel-agent 镜像；里面有逐行中文注释 |
| `docker-compose.yml` | 服务编排；每个配置项都有中文大白话说明 |
| `.env.example` | 环境变量模板；逐项解释每个变量含义 |
| `initdb/02-seed-demo-kb.sql` | 空库首次初始化时插入演示知识库 |
| `../migrations/000001_rag_baseline.sql` | 空库 schema（首次初始化挂载） |
| `../.dockerignore` | 控制 docker build 忽略哪些文件，避免把密钥/缓存打进镜像 |

## 7. 常见坑

1. **`POSTGRESQL_DSN` 写成 localhost**  
   在 app 容器里 localhost 不是数据库容器。应写服务名 `postgres`。

2. **宿主机 5432 / 8081 端口冲突**  
   改 `POSTGRES_HOST_PORT` / `APP_HOST_PORT`。

3. **改了 SQL 但数据库没变**  
   初始化脚本只在数据卷第一次创建时执行。需要重建空库时：

```bash
docker compose -f docker/docker-compose.yml --env-file docker/.env down -v
docker compose -f docker/docker-compose.yml --env-file docker/.env up -d --build
```

4. **没填 `EMBEDDING_API_KEY`**  
   compose 会直接失败，这是故意的，避免 app 反复重启。

5. **密钥误提交**  
   只提交 `.env.example`，真实 `docker/.env` 保持本地且被忽略。
