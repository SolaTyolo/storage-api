# Cursor Agent configuration

This repository configures [Cursor Agents](https://cursor.com/docs/agent/overview) via **AGENTS.md** and **Project Rules**.

## Files

| File | Purpose |
|------|---------|
| [AGENTS.md](../AGENTS.md) | Root agent instructions (build, architecture, guardrails) — read first |
| `.cursor/rules/*.mdc` | Cursor project rules (always-on + file-scoped) |

## Project rules (`.cursor/rules/`)

| Rule | Scope | Description |
|------|-------|-------------|
| `project.mdc` | Always apply | Core architecture and language rules |
| `engine.mdc` | `internal/engine/**` | Storage engine interface and registry |
| `preview-transform.mdc` | `internal/preview/**`, `internal/transform/**` | On-demand preview pipeline |

Nested `AGENTS.md` files apply when editing files in those directories.

## Nested AGENTS.md

| Path | Focus |
|------|-------|
| [deploy/AGENTS.md](../deploy/AGENTS.md) | Docker Compose and container builds |
| [internal/api/AGENTS.md](../internal/api/AGENTS.md) | Supabase HTTP handler patterns |

## Related docs

- [docs/ARCHITECTURE.md](../docs/ARCHITECTURE.md)
- [docs/TRANSFORM_BACKENDS.md](../docs/TRANSFORM_BACKENDS.md)
- [deploy/README.md](../deploy/README.md)
