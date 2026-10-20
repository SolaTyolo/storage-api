# obj-gateway playground

Browser UI to exercise `obj-gateway-sdk` against the local gateway + **RustFS** stack.

## Via Docker Compose (recommended)

From repo root:

```bash
make docker-up
```

Open http://localhost:5173

| Service | URL |
|---------|-----|
| Playground | http://localhost:5173 |
| Gateway | http://localhost:8080 |
| CDN | http://localhost:8081 |
| RustFS S3 | http://localhost:9000 |
| RustFS Console | http://localhost:9001 (`rustfsadmin` / `rustfsadmin`) |

## Local Vite (gateway already running)

```bash
cd packages/sdk && npm run build
cd ../playground && npm install && npm run dev
```
