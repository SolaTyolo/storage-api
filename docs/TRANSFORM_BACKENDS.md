# 开源免费交付中间件栈

Storage API 只存 **S3 原文件 + Postgres 元数据**；预览/缩略统一走 `GET /storage/v1/objects/{id}/image?w=&h=&c=...`。

## 组件（全部开源）

| 中间件 | 许可证 | 作用 | 端口 (compose) |
|--------|--------|------|----------------|
| [imgproxy](https://github.com/imgproxy/imgproxy) | MIT | 图片 `w/h/c/q/f`，S3 直读 | 8081 |
| [Gotenberg](https://github.com/gotenberg/gotenberg) | MIT | Office → PDF | 3000 |
| **preview-worker** | MIT | Poppler `pdftoppm`：PDF 页 → JPEG | 8090 |
| **内置 transform** | — | 视频 ffmpeg 截帧；PDF/Office 栅格化后再缩放 | API 内 |

Poppler 为 **GPL-2.0**（系统 `poppler-utils`），仅跑在独立 sidecar，不与 API 进程混合。

## 按 MIME 路由

```text
GET /objects/{id}/image?w=200&h=200&c=fill&page=1&dpi=150
        │
        ├─ image/*     ──► imgproxy（TRANSFORM_BACKEND=imgproxy）或内置 imaging
        ├─ video/*     ──► 内置 ffmpeg + imaging
        ├─ application/pdf ──► Poppler worker → imaging
        └─ Office/*    ──► Gotenberg → PDF → Poppler → imaging
```

其他类型（zip、纯文本等）：`415`，使用 `download-url`。

## 环境变量

```env
TRANSFORM_BACKEND=imgproxy   # 图片走 imgproxy；视频/PDF 仍用 API 管线
IMGPROXY_BASE_URL=http://imgproxy:8080
IMGPROXY_INSECURE=true       # 仅开发；生产应对 imgproxy URL 签名

GOTENBERG_URL=http://gotenberg:3000
POPPLER_WORKER_URL=http://preview-worker:8090
```

## 启动

```bash
docker compose up -d --build
```

- Playground: http://localhost:8080/playground/
- imgproxy: http://localhost:8081
- Gotenberg: http://localhost:3000/health

## 生产建议

1. imgproxy：关闭 `/insecure/`，配置 `IMGPROXY_KEY` / `IMGPROXY_SALT`，由 API 签发短时 URL。
2. Gotenberg / preview-worker：仅内网访问，不对公网暴露。
3. API 在生成 `/image` 前校验对象权限（bucket policy / JWT）。

## 未覆盖格式

- 音频波形、HEIC（部分可由 imgproxy 处理源图）
- 复杂 CAD、加密 PDF

可再增专用开源服务，由 API 增加路由即可，无需改 `storage.objects`  schema。
