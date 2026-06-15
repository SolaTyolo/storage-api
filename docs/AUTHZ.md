# External authorization (authz)

When `AUTHZ_HTTP_URL` is set, every authenticated `/storage/v1` request is checked against an external HTTP policy service **after** API key / JWT identity validation.

## Request

`POST {AUTHZ_HTTP_URL}` with JSON body:

```json
{
  "subject": {
    "type": "jwt",
    "role": "authenticated",
    "user_id": "uuid",
    "claims": { "role": "authenticated", "sub": "uuid" }
  },
  "resource": {
    "method": "GET",
    "path": "/storage/v1/object/authenticated/uploads/a.png",
    "action": "read",
    "bucket_id": "uploads",
    "object_path": "a.png"
  }
}
```

`subject.type` is `api_key`, `jwt`, or `anonymous`.

## Response

HTTP **2xx** with:

```json
{ "allowed": true }
```

HTTP **403** or `{ "allowed": false, "reason": "..." }` denies the request.

## Bypass (default)

| Env | Default | Effect |
|-----|---------|--------|
| `AUTHZ_BYPASS_API_KEY` | `true` | Static `API_KEY` / `API_KEYS` skip authz |
| `AUTHZ_BYPASS_SERVICE_ROLE` | `true` | JWT with `role=service_role` skips authz |

Public routes (`/object/public/`, `/render/image/public/`) never call authz.

## Go interface

```go
type Authorizer interface {
    Authorize(ctx context.Context, subject Subject, resource Resource) error
}
```

Implement `internal/authz.Authorizer` and wire via config, or use the built-in `authz.HTTP` client.
