# Docker 部署说明

根目录 `docker/` 独立保存 TravelAgent 的镜像构建和 Compose 编排；Go 源码位于同级 `backend/`，Vue 源码位于同级 `frontend/`。所有本页命令都从**仓库根目录**执行。

## 默认与可选服务

默认启动：

1. `postgres`：PostgreSQL 16 + pgvector。
2. `app`：TravelAgent Go 后端，默认使用 Docker 命名卷保存上传文件。

增加 `--profile s3` 时还会启动：

3. `minio`：S3 兼容对象存储。
4. `minio-init`：等待 MinIO 就绪并创建 bucket。

## 重要约束

- Go 程序只读取进程环境变量，不会自动读取 `.env`；`docker compose --env-file docker/.env` 负责注入。
- 应用启动不会自动执行数据库迁移。只有 PostgreSQL 数据卷首次创建时，官方入口才会执行 `backend/migrations/000001_rag_baseline.sql` 和 `docker/initdb/02-seed-demo-kb.sql`。
- baseline SQL 含重建语句，只能用于全新空数据库，不能对已有业务库重复执行。
- 上传前知识库必须存在；演示初始化会创建 `kb_demo_001`。

## 1. 准备环境变量

```bash
cp docker/env.example docker/.env
```

编辑 `docker/.env`，至少把 `EMBEDDING_API_KEY` 改成真实密钥。其他端口、数据库和对象存储配置的逐项说明见 `docker/env.example`。真实 `.env` 已被 Git 和 Docker 构建上下文忽略。

## 2. 启动默认栈

```bash
docker compose -f docker/docker-compose.yml --env-file docker/.env up -d --build
docker compose -f docker/docker-compose.yml --env-file docker/.env ps
curl http://localhost:8081/health
```

Compose 的构建上下文是仓库根目录，使用 `docker/Dockerfile`。Dockerfile 只复制 `backend/go.mod`、`backend/go.sum`、`backend/cmd/` 和 `backend/internal/`，不会把前端或仓库工具打进 Go 镜像。

## 3. 启动 MinIO / S3 栈

先在 `docker/.env` 中设置：

```env
RUSTFS_ENABLED=true
RUSTFS_BUCKET_NAME=travelagent
RUSTFS_ENDPOINT=http://minio:9000
RUSTFS_ACCESS_KEY=minioadmin
RUSTFS_SECRET_KEY=minioadmin
RUSTFS_PATH_STYLE=true
```

然后执行：

```bash
docker compose -f docker/docker-compose.yml --env-file docker/.env --profile s3 up -d --build
```

MinIO 控制台默认地址是 `http://localhost:9001`。示例账号密码仅适合本地开发，生产环境必须更换。

## 4. 演示上传

首次空库初始化会创建演示知识库 `kb_demo_001`：

```bash
curl -X POST "http://localhost:8081/api/knowledge/bases/kb_demo_001/documents/upload" \
  -F "file=@./backend/README.md"
```

## 5. 常用运维命令

```bash
# 查看后端日志
docker compose -f docker/docker-compose.yml --env-file docker/.env logs -f app

# 停止容器并保留数据卷
docker compose -f docker/docker-compose.yml --env-file docker/.env down

# 停止容器并删除数据库、上传文件和 MinIO 数据
docker compose -f docker/docker-compose.yml --env-file docker/.env down -v

# 只重建后端镜像并重启 app
docker compose -f docker/docker-compose.yml --env-file docker/.env up -d --build app
```

## 6. 文件职责

| 路径 | 作用 |
|---|---|
| `docker/Dockerfile` | 多阶段构建 `backend/` 中的 Go 服务 |
| `docker/docker-compose.yml` | 编排 PostgreSQL、后端和可选 MinIO |
| `docker/env.example` | Compose 环境变量模板 |
| `docker/initdb/02-seed-demo-kb.sql` | 空库首次初始化时插入演示知识库 |
| `backend/migrations/000001_rag_baseline.sql` | 全新空库 baseline schema |
| `.dockerignore` | 根级构建上下文排除规则 |

## 7. 常见问题

1. 容器内数据库主机必须写 Compose 服务名 `postgres`，不能写 `localhost`。
2. 宿主机端口冲突时修改 `APP_HOST_PORT`、`POSTGRES_HOST_PORT` 或 MinIO 端口。
3. 初始化 SQL 只在数据卷第一次创建时执行；确实要重建本地空库时才使用 `down -v`。
4. 未设置 `EMBEDDING_API_KEY` 时 Compose 会直接拒绝生成配置，避免后端无效重启。
5. 若 Docker Desktop/daemon 未启动，只能做 `docker compose config` 静态校验，不能实际拉镜像或启动服务。
