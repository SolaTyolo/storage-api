# storage-api（obj-gateway）

面向 S3 兼容对象存储（RustFS / MinIO / OSS / COS）的**预签名 URL 网关**。Go 网关不接收文件二进制，前端直传对象存储。模块路径：`github.com/SolaTyolo/storage-api`。

[English](./README.md)

## 特性

- PutObject 预签名 + S3 Multipart（创建 / 分片 / 合并 / 终止）
- JWT 鉴权，可选 bucket / key 前缀隔离
- 业务元数据写入 `x-amz-meta-*`，基础信息走 `HeadObject`（无独立元数据库）
- 前端 SDK `obj-gateway-sdk`
- Go SDK `packages/go-sdk`（业务后端；同样走预签名，不经网关传文件体）
- 开发环境：**RustFS** + Nginx CDN + 网关 + Playground 一键启动

## 快速开始

```bash
make docker-up
# Playground http://localhost:5173
# API        http://localhost:8080
# CDN        http://localhost:8081
# RustFS     http://localhost:9000（控制台 :9001）
```

健康检查：`GET http://localhost:8080/health`

## 仓库结构

```
packages/gateway/      Go 网关
packages/sdk/          TypeScript SDK
packages/go-sdk/       Go 后端 SDK
packages/playground/   浏览器上传调试页
docker/                Compose（RustFS）/ Dockerfile / Nginx
docs/                  架构、OpenAPI、SDK 文档
```

## 配置

| 变量 | 说明 |
|------|------|
| `JWT_SECRET` | HS256 密钥（为空则开发模式放行） |
| `BUCKET_CONFIG` | 桶配置 JSON（endpoint、AK/SK、CDN） |

本地 Docker 基础镜像可通过根目录 `.env` 覆盖（见 `.env.example` 中的 `GOLANG_IMAGE` / `ALPINE_IMAGE` 等）。推送到腾讯云 TCR：配置仓库 Secrets `TCR_USERNAME` / `TCR_PASSWORD`，在 `main` 合入或打 `gateway-v*` 标签时由 Actions 构建并推送 gateway 镜像。

## 业务集成

业务后端只需持久化 `bucket + key`，推荐使用 Go SDK（[docs/go-sdk-usage.md](docs/go-sdk-usage.md)）：

- 上传：`Upload` / `MultipartUpload`（网关签发 URL，字节直传对象存储）
- 公开资源：使用返回的 `cdnUrl`
- 私有资源：`DownloadURL` / `Download`
- 元数据：`Head`

更多见 [docs/architecture.md](docs/architecture.md)、[docs/sdk-usage.md](docs/sdk-usage.md)。
