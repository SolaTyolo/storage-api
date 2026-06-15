# storage-api 项目分析与改进建议

> 分析时间：2026-06（第六轮）。TUS **不实现**；已添加 authz 接口、跨引擎 copy、契约测试、异步 Preview、侧车鉴权。

## 1. 已落地能力（累计）

| 类别 | 能力 |
|------|------|
| **鉴权** | API Key / JWT HS256 |
| **授权 (authz)** | `Authorizer` 接口 + `AUTHZ_HTTP_URL` 外部 HTTP 策略 |
| **数据面** | S3 presign；公开/认证 302 |
| **跨引擎** | copy/move 网关流式中转（`destinationBucket` 支持 `engine:bucket`） |
| **Preview** | 同步 + `PREVIEW_ASYNC` 异步任务轮询 |
| **侧车** | `SIDECAR_API_TOKEN` → Gotenberg / preview-worker |
| **质量** | Go 契约测试 + 可选 Node `tests/contract` |

## 2. 明确不实现

- **TUS 断点续传**

## 3. 剩余差距

| 项 | 说明 |
|----|------|
| **OpenTelemetry** | 仅有 Prometheus；见下文说明 |
| **storage-js 全量 SDK 测试** | 可选 Node 契约需 live server |
| **跨引擎大文件** | 流式中转占 API 带宽 |

## 4. OpenTelemetry 说明

Prometheus metrics 回答「QPS、延迟分位、错误率」——适合单机/单服务监控。

OpenTelemetry **tracing** 回答「一次请求经过了哪些 hop、每段耗时」——例如：

`客户端 → storage-api → S3 Head → Gotenberg 转换 → Poppler 渲染 → 返回`

当 Preview 链路变长、或多引擎并发故障时，trace 比 metrics 更快定位慢在哪个环节。**不是必须**，而是运维复杂度上去后的增强项。

## 5. 配置速查（新增）

| 变量 | 用途 |
|------|------|
| `AUTHZ_HTTP_URL` | 外部 HTTP 授权服务 |
| `AUTHZ_BYPASS_API_KEY` | API Key 跳过 authz（默认 true） |
| `AUTHZ_BYPASS_SERVICE_ROLE` | service_role JWT 跳过（默认 true） |
| `PREVIEW_ASYNC` | PDF/Office 异步渲染 |
| `SIDECAR_API_TOKEN` | Gotenberg / preview-worker Bearer |

## 6. 相关文档

- [AUTHZ.md](./AUTHZ.md) — 外部 HTTP 授权协议
- [CROSS_ENGINE.md](./CROSS_ENGINE.md) — 跨引擎 copy/move 方案
- [ARCHITECTURE.md](./ARCHITECTURE.md)
