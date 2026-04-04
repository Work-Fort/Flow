# Flow

Configurable workflow engine for the [WorkFort](https://github.com/Work-Fort) platform.

Flow manages business processes as directed graphs of steps that work items flow through. It is process-agnostic — SDLC, incident response, onboarding, and other workflows are all configurations of the same engine.

## How it works

Flow provides three layers:

- **Templates** define a process blueprint: steps (graph nodes), transitions (graph edges), role permissions, and integration hooks
- **Instances** bind a template to a team and its integrations (Git forge, chat channels)
- **Work items** flow through an instance, moving between steps as agents trigger transitions

```
Template (blueprint)
  |
  +-- Instance (config container, bound to team + integrations)
        |
        +-- Work Item --> [Step A] --transition--> [Step B] --> ...
        +-- Work Item --> [Step C] --transition--> [Step D] --> ...
```

Transitions are always intentional acts — triggered by an agent, a human, or an explicit automation rule. The engine validates and records transitions but never auto-advances work items.

### Integration model

Flow connects to external services through port interfaces with swappable adapters:

| Port | Adapters | Purpose |
|------|----------|---------|
| **Git Forge** | Combine, GitHub | Issues, commits, status sync |
| **Chat** | Sharkfin | Bot commands, notifications, audit trail |
| **Identity** | Hive | Agent/role/team resolution |

Sharkfin is the primary interaction surface. Flow operates a bot identity that receives structured commands and posts state changes to channels.

### Guard expressions

Transition conditions are expressed in [CEL (Common Expression Language)](https://cel.dev/), a non-Turing-complete expression language used by Kubernetes and Google Cloud IAM:

```
assignee.role_id == "reviewer" && item.fields.tests_passing == true
```

## MCP tools

| Tool | Description |
|------|-------------|
| `list_templates` | List available workflow templates |
| `get_template` | Get template details |
| `create_instance` | Start a workflow from a template |
| `list_instances` | List active workflow instances |
| `create_work_item` | Add a work item to an instance |
| `list_work_items` | List work items with filters |
| `get_work_item` | Get work item details and history |
| `transition_work_item` | Move a work item to the next step |
| `approve_work_item` | Approve at a gate step |
| `reject_work_item` | Reject at a gate step |
| `assign_work_item` | Assign an agent to a work item |
| `get_instance_status` | Dashboard view of an instance |

## Quick start

### Prerequisites

- [Go 1.26+](https://go.dev/)
- [mise](https://mise.jdx.dev/) (`mise install` to set up tooling)

### Build and run

```bash
mise run build
./build/flow daemon
```

### Connect via MCP

```bash
./build/flow mcp-bridge
```

### Add to Claude Code

```bash
claude mcp add flow -- flow mcp-bridge
```

## Architecture

See [docs/2026-04-03-flow-architecture-design.md](docs/2026-04-03-flow-architecture-design.md) for the full design document.

### Project layout

```
cmd/
  daemon/        -- HTTP server, systemd service
  mcp-bridge/    -- stdio-to-HTTP MCP bridge
  admin/         -- CLI admin commands
domain/          -- Core types, port interfaces, business rules
infra/
  combine/       -- Combine Git forge adapter
  github/        -- GitHub Git forge adapter (future)
  sharkfin/      -- Sharkfin chat/bot adapter
  hive/          -- Hive identity adapter
  sqlite/        -- SQLite store
  postgres/      -- PostgreSQL store
  httpapi/       -- REST API handlers
  mcp/           -- MCP tool handlers
```

## License

GPL-v2.0-Only
