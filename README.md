# storage-api（Go + RustFS）

Supabase Storage 风格的元数据 + S3 原文件存储；交付采用 **Cloudinary 式 URL 参数**（`w`、`h`、`c`、`page`、`dpi`），由开源中间件按需渲染，**不预生成衍生文件**。

| 组件 | 用途 |
|------|------|
| **imgproxy** (MIT) | 图片变换 |
| **Gotenberg** (MIT) | Office → PDF |
| **preview-worker** (Poppler) | PDF 页 → JPEG |
| 内置 ffmpeg / imaging | 视频截帧、栅格图再缩放 |

## 快速启动

```bash
docker compose up -d --build
```

- Playground: http://localhost:8080/playground/
- 变换示例: `GET /storage/v1/objects/{id}/image?w=320&h=200&c=fill&q=85`

```bash
cp .env.example .env
go run ./cmd/server
```

## 目录结构

```
cmd/server/
internal/api/          # HTTP + playground
internal/transform/    # 按需图像变换
migrations/
docker-compose.yml
```

详见 [docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md)、[docs/TRANSFORM_BACKENDS.md](./docs/TRANSFORM_BACKENDS.md)。

## 许可证

MIT
