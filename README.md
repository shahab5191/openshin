# openshin

Local layered-memory daemon for agents. Records every conversation, distils it
into decaying mid-term facts, durable long-term knowledge, and procedural skill
memory, and lets an orchestrator agent (and its fanout subagents) recall what has
been done, what was decided, and what might conflict — across sessions.

Pull-only: openshin never injects context into an agent; agents retrieve on
demand through MCP tools.

See [docs/architecture.md](docs/architecture.md) for the full design.

## Status

Phase 0 — scaffold only. No memory logic is implemented yet.

## Layout

```
cmd/openshin/         daemon binary (serve / mcp / context)
internal/db/          SQLite connect + sqlc-generated queries
internal/db/queries/  sqlc query files (phase 1+)
internal/store/       repository layer (phase 1+)
internal/memory/      memory layers (phase 2+)
internal/pipeline/    promotion engine (phase 3+)
internal/llm/         provider abstraction (phase 1+)
internal/mcp/         MCP tool server (phase 6)
internal/ingest/      transcript tailer + HTTP ingest (phase 1)
migrations/           goose SQLite migrations
docs/                 design docs
```

## Develop

```sh
make migrate-up   # create the schema (phase 1+)
make sqlc         # generate typed queries
make build        # go build ./...
make test         # go test ./...
make vet          # go vet ./...
```
