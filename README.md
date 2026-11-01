# openshin

A unified, layered memory for local agents — one daemon, one SQLite file, that
every agent on the machine reads and writes. Over time it becomes the knowledge
hub: the place agents go to recall what has been done, what was decided, and
what works, so each new task starts from everything already learned instead of
from zero.

It records every conversation and distils it into decaying mid-term facts,
durable long-term knowledge, and procedural skill memory, so an orchestrator
agent and its fanout subagents work more efficiently and accurately as their
shared context grows.

Pull-only: openshin never injects context into an agent; agents retrieve on
demand through MCP tools.

See [docs/architecture.md](docs/architecture.md) for the full design.

## Status

Local system implemented (phases 0–6): raw log + ingest, short-term context,
mid-term facts, long-term knowledge, skill memory, and MCP/hooks alignment.
The fleet (shared backend, many agents) is the remaining, not-yet-started phase.

## Layout

```
cmd/openshin/         daemon binary (serve / tail / mcp / hook)
internal/db/          SQLite connect + sqlc-generated queries
internal/db/queries/  sqlc query files
internal/store/       repository layer (conversations, segments, facts, skills)
internal/memory/      memory layers (short-term, mid-term, long-term, skill, retrieval)
internal/llm/         provider (Gemini: summarise, extract, embed)
internal/mcp/         MCP tool server
internal/ingest/      transcript tailer + HTTP ingest
internal/api/         HTTP API (ingest, read, context, search)
migrations/           goose SQLite migrations
docs/                 design + per-layer specs
```

## Develop

```sh
make migrate-up   # create the schema
make sqlc         # generate typed queries
make build        # go build ./...
make test         # go test ./...
make vet          # go vet ./...

go run ./cmd/openshin serve    # run the daemon (HTTP API + background pipeline)
go run ./cmd/openshin mcp      # MCP server (claude mcp add openshin -- ...)
```

## License

[GPL-3.0](LICENSE)
