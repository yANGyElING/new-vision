# new-vision

阶段一自治 Node 的工程骨架。当前交付包含 NodeApp Go HTTP 进程、Vue 管理端、PostgreSQL/Redis 运行依赖、显式数据库迁移、node-app 设备权威和内部设备 API、HA1 派生、Access JSON-RPC 同步 outbox、NodeApp Redis 运行状态投影，以及固定 Kamailio 6.1.3 构建/配置和自定义模块。Kamailio 模块已实现 profile/event 控制面、REGISTER/Digest、KeepAlive 和 runtime 超时逻辑，但真实镜像编译、SIP/RPC 闭环和媒体编排仍未在本机完成。

## 当前状态

已验证：

- Go `go1.26.5 windows/amd64`、Node `v22.23.2`、npm `10.9.8`、Docker CLI `29.7.2`、Compose `v5.4.0` 已安装。
- Go 测试、静态检查、两个入口构建、前端 `npm ci`、类型检查和生产构建通过。
- `docker compose --env-file .env.example config --quiet` 通过。
- ZLMediaKit 使用官方 `zlmediakit/zlmediakit:master` 的固定 OCI manifest digest，并已核对 Linux/amd64 子 manifest。

待本机 Docker daemon 可用后执行：

- Compose 镜像构建、容器启动、统一入口和所有容器健康检查。
- PostgreSQL/Redis 降级恢复、迁移版本持久化和浏览器端到端验收。

当前机器只有 Docker CLI/Compose，Docker Desktop、WSL 发行版和 `docker_engine` daemon 未安装或未运行，因此上述运行验收不能在本机完成。

## 快速开始

需要 Go、Node/npm 和 Docker Compose。配置使用本地 `.env`，不要把生产凭据写入仓库。

PowerShell：

```powershell
Copy-Item .env.example .env
Set-Location web
npm ci
npm run typecheck
npm run build
Set-Location ..
go test ./...
go vet ./...
docker compose config --quiet
docker compose up --build -d
docker compose ps
```

Linux/macOS：

```sh
cp .env.example .env
npm ci --prefix web
npm run typecheck --prefix web
npm run build --prefix web
go test ./...
go vet ./...
docker compose config --quiet
docker compose up --build -d
docker compose ps
```

默认 HTTP 入口为 `http://localhost:8080/`，由 `reverse-proxy` 发布；摄像头 SIP UDP 通过 `${NV_SIP_PORT}`（默认 `5060`）发布。Kamailio 的 `8090/tcp` Access API 只在 Compose 网络内暴露，RTSP、RTMP、RTP、WebRTC 和 ZLMediaKit 管理端口均未发布。

## 健康检查

- `GET /api/health`：统一入口公开的非敏感健康摘要。
- `GET /livez`：Go 进程存活检查，仅容器内部可达。
- `GET /readyz`：Go 进程就绪检查，仅容器内部可达。
- `GET /metrics`：Prometheus 文本指标，仅容器内部可达。

`node-app` 只有在 PostgreSQL 和 Redis 都可探测时返回 `200`/`ready`；任一依赖异常时仍保持进程存活，并返回 `503`/`not_ready`。依赖恢复后无需重启应用即可恢复就绪。

## 迁移

应用启动不会自动修改数据库结构。迁移是一次性 Compose 工具；当前迁移创建设备权威表和 Access profile outbox：

```sh
docker compose run --rm migrate up
docker compose run --rm migrate version
docker compose run --rm migrate down 1
```

PostgreSQL 使用命名卷 `new-vision-postgres-data`。普通 `docker compose down` 保留数据；确认数据可删除时才执行：

```sh
docker compose down -v
```

## 服务边界

- `node-app`：配置校验、结构化日志、HTTP 生命周期、健康/指标、PostgreSQL 设备业务权威、内部设备 API、HA1 派生、profile outbox、Access JSON-RPC 同步和 NodeApp Redis 状态投影。
- `node-access`：基于 Kamailio 6.1.3 源码构建的 SIP/Access 运行单元；自定义模块已实现嵌套 JSON-RPC profile/event 控制面、MD5 Digest REGISTER、KeepAlive XML 和 runtime 超时，真实镜像编译与 Compose 闭环仍受本机 Docker daemon 缺失阻塞。
- `node-web`：Vue 3 + TypeScript 状态页、认证未启用占位页和 404 页。
- `reverse-proxy`：Nginx 统一 HTTP 入口，`/api/*` 转发到 `node-app`，其余路径转发到 `node-web`。
- `migrate`：显式 `golang-migrate` 工具；基线迁移不创建业务表。

## 版本记录

| 组件 | 固定引用 | 官方来源 |
| --- | --- | --- |
| Go builder | `golang:1.26.0-alpine3.22` | [Docker Official Image](https://hub.docker.com/_/golang) |
| Node builder | `node:22.22.0-alpine3.22` | [Docker Official Image](https://hub.docker.com/_/node) |
| Alpine runtime | `alpine:3.22.1` | [Docker Official Image](https://hub.docker.com/_/alpine) |
| Nginx | `nginx:1.29.1-alpine3.22` | [Docker Official Image](https://hub.docker.com/_/nginx) |
| PostgreSQL | `postgres:17.6-alpine3.22` | [Docker Official Image](https://hub.docker.com/_/postgres) |
| Redis | `redis:8.2.1-alpine3.22` | [Docker Official Image](https://hub.docker.com/_/redis) |
| golang-migrate | `migrate/migrate:v4.18.3` | [migrate/migrate](https://hub.docker.com/r/migrate/migrate) |
| ZLMediaKit | `zlmediakit/zlmediakit:master@sha256:2bb6a49f61944bc2437da5d670581837cdf41cf1ed045b80f1590b82499c4b98` | [ZLMediaKit/ZLMediaKit](https://github.com/ZLMediaKit/ZLMediaKit) |

以上 registry manifest 于 2026-08-15 核对存在；ZLMediaKit 同时核对了 Linux/amd64 子 manifest。其 digest 固定后仍需在具备 Docker daemon 的环境完成实际拉取和容器启动验证。

## Git 和秘密

仓库已执行本地 `git init`，没有配置远程、没有 commit、没有 push。`.env`、证书、密钥、构建产物和依赖目录已忽略；`.env.example` 中的值只适用于本地示例，不能用于生产。
