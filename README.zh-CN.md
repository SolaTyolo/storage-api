# storage-api（Go + RustFS）

与 Supabase Storage SDK 兼容的 HTTP API，直接对接物理存储引擎（S3 兼容）。无 PostgreSQL，无逻辑 bucket。

[English](./README.md)

- **多引擎**：YAML 配置 + `STORAGE_DEFAULT_ENGINE` 环境变量
- **Bucket 路由**：`engine:bucket`（如 `rustfs:uploads`），省略 engine 则使用默认引擎
- **Preview**：图片 / 视频 / PDF / Office 按需渲染（imgproxy、Gotenberg、Poppler、ffmpeg）

## 快速启动

```bash
docker compose -f deploy/docker-compose.yml up -d --build
```

- 变换示例: `GET /storage/v1/render/image/public/uploads/photo.png?width=320&height=200&resize=cover`

```bash
cp .env.example .env
go run ./cmd/server
```

## 配置

`config/storage.yaml`：

```yaml
default_engine: rustfs
engines:
  rustfs:
    type: s3
    endpoint: http://localhost:9000
    region: us-east-1
    access_key_id: rustfsadmin
    secret_access_key: rustfsadmin
    path_style: true
```

环境变量：

| 变量 | 说明 |
|------|------|
| `STORAGE_CONFIG_PATH` | YAML 配置文件路径 |
| `STORAGE_DEFAULT_ENGINE` | 覆盖默认引擎 |
| `API_KEY` | 服务端 API 密钥；设置后 `/storage/v1` 需携带 |
| `API_KEYS` | 逗号分隔多密钥 |
| `JWT_SECRET` | Supabase HS256 JWT 密钥；支持 `Authorization: Bearer <jwt>` |
| `ALLOW_PRESIGNED_UPLOAD` | `false` 禁用 S3 presigned 上传 |
| `AUTH_DOWNLOAD_MODE` | `proxy` 或 `redirect` |
| `IMGPROXY_KEY` / `IMGPROXY_SALT` | 生产 imgproxy 签名（`IMGPROXY_INSECURE=false` 时必填） |
| `AUTHZ_HTTP_URL` | 外部 HTTP 授权服务（见 [docs/AUTHZ.md](./docs/AUTHZ.md)） |
| `PREVIEW_ASYNC` | PDF/Office 异步预览 |
| `SIDECAR_API_TOKEN` | Gotenberg / preview-worker Bearer 鉴权 |

## Supabase SDK 用法

```typescript
const supabase = createClient(url, key, {
  global: { fetch: customFetch },
});
// bucket 支持 engine:bucket 格式
await supabase.storage.from('rustfs:uploads').upload('a.png', file);
await supabase.storage.from('uploads').download('a.png');
```

## 目录结构

```
cmd/server/
internal/api/          # Supabase Storage HTTP API
internal/engine/       # 多引擎注册与 S3 实现
internal/transform/    # 按需图像变换
internal/preview/      # PDF/Office 预览
config/storage.yaml    # 本地 / 非 Docker 存储配置
deploy/                # Docker Compose、Dockerfile、容器配置
```

详见 [docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md)、[docs/TRANSFORM_BACKENDS.md](./docs/TRANSFORM_BACKENDS.md)、[docs/ANALYSIS.zh-CN.md](./docs/ANALYSIS.zh-CN.md)。

## 许可证

MIT
