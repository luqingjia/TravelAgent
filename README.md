# TravelAgent

TravelAgent 是一个前后端分离目录、Docker 独立编排的单仓库项目。

## 目录结构

```text
TravelAgent/
├── backend/                  # Go 1.26 后端模块
├── frontend/                 # Vue 3 + TypeScript + Vite 前端
├── docker/                   # Dockerfile、Compose、环境变量模板和初始化数据
├── .dockerignore             # 根级 Docker 构建上下文排除规则
├── .trellis/                 # 项目任务、规范与工作区
├── AGENTS.md                 # 仓库级开发约束
└── CLAUDE.md                 # AI 开发上下文
```

## 后端

详细架构、环境变量、数据库、API 和运行说明见 [`backend/README.md`](backend/README.md)。

```powershell
cd backend
go test ./...
go vet ./...
go build -o ../.trellis/workspace/bin/travel-agent.exe ./cmd/travel-agent
go run ./cmd/travel-agent
```

`backend/.env.example` 只是模板，Go 程序不会自动加载 `.env`。数据库迁移位于 `backend/migrations/`，应用启动不会自动执行迁移。

## 前端

前端说明见 [`frontend/README.md`](frontend/README.md)。

```powershell
cd frontend
pnpm install
pnpm dev
```

常用质量命令：

```powershell
pnpm lint
pnpm test:unit
pnpm build
pnpm test:e2e
```

## Docker

Docker 资产独立放在根目录 [`docker/`](docker/README.md)。以下命令均从仓库根目录执行：

```bash
cp docker/env.example docker/.env
# 编辑 docker/.env，至少设置真实的 EMBEDDING_API_KEY
docker compose -f docker/docker-compose.yml --env-file docker/.env up -d --build
```

默认启动 PostgreSQL/pgvector 和 Go 后端；增加 `--profile s3` 可同时启动 MinIO。Compose 从仓库根目录构建镜像，但 Dockerfile 只复制 `backend/` 中的 Go 依赖清单与生产源码。

## 开发约定

- Go 命令在 `backend/` 执行，Node.js 命令在 `frontend/` 执行。
- Docker Compose、Git、Trellis 和仓库级文档检查在仓库根目录执行。
- 不提交 `.env`、API Key、数据库密码、对象存储密钥、本地数据、缓存或构建产物。
- 修改后端时遵守 `AGENTS.md` 与 `.trellis/spec/backend/` 中的依赖边界和质量门。
