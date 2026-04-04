# Flow Phase 1 — Core Engine Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the Flow core workflow engine: project skeleton, domain layer with CEL guard support, SQLite store, REST API, MCP bridge, and SDLC template seed.

**Reference implementation:** Hive (`/home/kazw/Work/WorkFort/hive/lead/`) — Flow follows the same Go patterns exactly. Read Hive's source before implementing each chunk.

**Architecture:** Single Go binary (`flow`), three modes: `flow daemon` (HTTP), `flow mcp-bridge` (stdio-to-HTTP), `flow admin` (CLI admin). Hexagonal: `internal/domain/` for ports; `internal/infra/` for adapters.

**Go module:** `github.com/Work-Fort/Flow`

**Out of scope (Phase 2+):** Sharkfin integration, Hive identity adapter, Combine/GitHub adapters, PostgreSQL, parallel fork/join, SLA tracking.

---

## Chunk 1: Build Tooling and Project Skeleton

### Task 1: mise.toml, .gitignore, Go module, main.go

**Files:** `mise.toml`, `.gitignore`, `go.mod`, `main.go`

- [ ] **Step 1: Create mise.toml** — follow `hive/lead/mise.toml` exactly, substituting `hive` → `flow`, port 17000 → 17200, module path `github.com/Work-Fort/Flow`.

```toml
# SPDX-License-Identifier: GPL-2.0-only
[tools]
go = "1.26.0"

[tasks.build]
description = "Build the flow binary"
run = "go build -o build/flow ."

[tasks.test]
description = "Run unit tests"
run = "go test ./..."

[tasks.lint]
description = "Run go vet"
run = "go vet ./..."

[tasks.clean]
description = "Remove build artifacts"
run = "rm -rf build/"
```

- [ ] **Step 2: Create `.gitignore`** — `build/`

- [ ] **Step 3: Initialize Go module and create main.go**

```bash
mise install
go mod init github.com/Work-Fort/Flow
go mod edit -go=1.26
```

`main.go`:
```go
// SPDX-License-Identifier: GPL-2.0-only
package main

import "github.com/Work-Fort/Flow/cmd"

func main() { cmd.Execute() }
```

- [ ] **Step 4: Verify** — `go mod tidy && go build ./...` exits 0.

- [ ] **Step 5: Commit** — `chore: initialize Flow Go module with mise build tooling`

---

### Task 2: Config package and Cobra root command

**Files:** `internal/config/config.go`, `cmd/root.go`

- [ ] **Step 1: Create `internal/config/config.go`** — follow `hive/lead/internal/config/config.go` exactly. Change:
  - `EnvPrefix = "FLOW"`
  - `DefaultPort = 17200`
  - `ConfigDir/StateDir` use `"flow"` subdirectory
  - Remove `max-role-depth` default; add `passport-url` and `passport-token` defaults (empty string)
  - Remove `max-role-depth` from `BindFlags`

- [ ] **Step 2: Create `cmd/root.go`** — follow `hive/lead/cmd/root.go` exactly. Change:
  - Binary name `"flow"`, short description `"Flow workflow engine daemon"`
  - Import path `github.com/Work-Fort/Flow/internal/config`
  - Subcommand registrations added in later tasks

- [ ] **Step 3: Verify** — `go mod tidy && go build -o build/flow . && ./build/flow --version` outputs `flow version dev`.

- [ ] **Step 4: Commit** — `feat: add config package and Cobra root command`

---

## Chunk 2: Domain Layer

### Task 3: Entity types

**Files:** `internal/domain/types.go`

The domain package has zero external dependencies (stdlib only: `time`, `encoding/json`).

- [ ] **Step 1: Create `internal/domain/types.go`** with the following types. All fields must match the design doc exactly.

```go
// SPDX-License-Identifier: GPL-2.0-only

// Package domain defines core types and port interfaces for Flow.
// This package has zero infrastructure dependencies.
package domain

import (
	"encoding/json"
	"time"
)

type StepType      string
type ApprovalMode  string
type ApprovalDecision string
type InstanceStatus string
type Priority      string

const (
	StepTypeTask StepType = "task"
	StepTypeGate StepType = "gate"

	ApprovalModeAny       ApprovalMode = "any"
	ApprovalModeUnanimous ApprovalMode = "unanimous"

	ApprovalDecisionApproved ApprovalDecision = "approved"
	ApprovalDecisionRejected ApprovalDecision = "rejected"

	InstanceStatusActive    InstanceStatus = "active"
	InstanceStatusPaused    InstanceStatus = "paused"
	InstanceStatusCompleted InstanceStatus = "completed"
	InstanceStatusArchived  InstanceStatus = "archived"

	PriorityCritical Priority = "critical"
	PriorityHigh     Priority = "high"
	PriorityNormal   Priority = "normal"
	PriorityLow      Priority = "low"
)

type WorkflowTemplate struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	Version          int               `json:"version"`
	Steps            []Step            `json:"steps"`
	Transitions      []Transition      `json:"transitions"`
	RoleMappings     []RoleMapping     `json:"role_mappings"`
	IntegrationHooks []IntegrationHook `json:"integration_hooks"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

type Step struct {
	ID         string          `json:"id"`
	TemplateID string          `json:"template_id"`
	Key        string          `json:"key"`
	Name       string          `json:"name"`
	Type       StepType        `json:"type"`
	Position   int             `json:"position"`
	Approval   *ApprovalConfig `json:"approval,omitempty"`
}

type ApprovalConfig struct {
	Mode              ApprovalMode `json:"mode"`
	RequiredApprovers int          `json:"required_approvers"`
	ApproverRoleID    string       `json:"approver_role_id"`
	RejectionStepID   string       `json:"rejection_step_id,omitempty"`
}

type Transition struct {
	ID             string `json:"id"`
	TemplateID     string `json:"template_id"`
	Key            string `json:"key"`
	Name           string `json:"name"`
	FromStepID     string `json:"from_step_id"`
	ToStepID       string `json:"to_step_id"`
	Guard          string `json:"guard,omitempty"`
	RequiredRoleID string `json:"required_role_id,omitempty"`
}

type RoleMapping struct {
	ID             string   `json:"id"`
	TemplateID     string   `json:"template_id"`
	StepID         string   `json:"step_id"`
	RoleID         string   `json:"role_id"`
	AllowedActions []string `json:"allowed_actions"`
}

type IntegrationHook struct {
	ID           string          `json:"id"`
	TemplateID   string          `json:"template_id"`
	TransitionID string          `json:"transition_id"`
	Event        string          `json:"event"`
	AdapterType  string          `json:"adapter_type"`
	Action       string          `json:"action"`
	Config       json.RawMessage `json:"config,omitempty"`
}

type WorkflowInstance struct {
	ID                 string              `json:"id"`
	TemplateID         string              `json:"template_id"`
	TemplateVersion    int                 `json:"template_version"`
	TeamID             string              `json:"team_id"`
	Name               string              `json:"name"`
	Status             InstanceStatus      `json:"status"`
	IntegrationConfigs []IntegrationConfig `json:"integration_configs,omitempty"`
	CreatedAt          time.Time           `json:"created_at"`
	UpdatedAt          time.Time           `json:"updated_at"`
}

type IntegrationConfig struct {
	ID          string          `json:"id"`
	InstanceID  string          `json:"instance_id"`
	AdapterType string          `json:"adapter_type"`
	Config      json.RawMessage `json:"config"`
}

type WorkItem struct {
	ID              string          `json:"id"`
	InstanceID      string          `json:"instance_id"`
	Title           string          `json:"title"`
	Description     string          `json:"description"`
	CurrentStepID   string          `json:"current_step_id"`
	AssignedAgentID string          `json:"assigned_agent_id,omitempty"`
	Priority        Priority        `json:"priority"`
	Fields          json.RawMessage `json:"fields,omitempty"`
	ExternalLinks   []ExternalLink  `json:"external_links,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type ExternalLink struct {
	ID          string `json:"id"`
	WorkItemID  string `json:"work_item_id"`
	ServiceType string `json:"service_type"`
	Adapter     string `json:"adapter"`
	ExternalID  string `json:"external_id"`
	URL         string `json:"url,omitempty"`
}

type TransitionHistory struct {
	ID           string    `json:"id"`
	WorkItemID   string    `json:"work_item_id"`
	FromStepID   string    `json:"from_step_id"`
	ToStepID     string    `json:"to_step_id"`
	TransitionID string    `json:"transition_id"`
	TriggeredBy  string    `json:"triggered_by"`
	Reason       string    `json:"reason,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
}

type Approval struct {
	ID         string           `json:"id"`
	WorkItemID string           `json:"work_item_id"`
	StepID     string           `json:"step_id"`
	AgentID    string           `json:"agent_id"`
	Decision   ApprovalDecision `json:"decision"`
	Comment    string           `json:"comment,omitempty"`
	Timestamp  time.Time        `json:"timestamp"`
}
```

- [ ] **Step 2: Commit** — `feat(domain): add all entity types`

---

### Task 4: Domain errors and port interfaces

**Files:** `internal/domain/errors.go`, `internal/domain/ports.go`

- [ ] **Step 1: Create `internal/domain/errors.go`**

```go
// SPDX-License-Identifier: GPL-2.0-only
package domain

import "errors"

var (
	ErrNotFound          = errors.New("not found")
	ErrAlreadyExists     = errors.New("already exists")
	ErrHasDependencies   = errors.New("has dependencies")
	ErrInvalidGuard      = errors.New("invalid guard expression")
	ErrGuardDenied       = errors.New("transition guard denied")
	ErrInvalidTransition = errors.New("invalid transition")
	ErrNotAtGateStep     = errors.New("work item is not at a gate step")
	ErrPermissionDenied  = errors.New("permission denied")
)
```

- [ ] **Step 2: Create `internal/domain/ports.go`**

```go
// SPDX-License-Identifier: GPL-2.0-only
package domain

import (
	"context"
	"io"
)

type TemplateStore interface {
	CreateTemplate(ctx context.Context, t *WorkflowTemplate) error
	GetTemplate(ctx context.Context, id string) (*WorkflowTemplate, error)
	ListTemplates(ctx context.Context) ([]*WorkflowTemplate, error)
	UpdateTemplate(ctx context.Context, t *WorkflowTemplate) error
	DeleteTemplate(ctx context.Context, id string) error
}

type InstanceStore interface {
	CreateInstance(ctx context.Context, i *WorkflowInstance) error
	GetInstance(ctx context.Context, id string) (*WorkflowInstance, error)
	ListInstances(ctx context.Context, teamID string) ([]*WorkflowInstance, error)
	UpdateInstance(ctx context.Context, i *WorkflowInstance) error
}

type WorkItemStore interface {
	CreateWorkItem(ctx context.Context, w *WorkItem) error
	GetWorkItem(ctx context.Context, id string) (*WorkItem, error)
	// ListWorkItems filters by instanceID (required), stepID, agentID, priority (all optional except instanceID).
	ListWorkItems(ctx context.Context, instanceID, stepID, agentID string, priority Priority) ([]*WorkItem, error)
	UpdateWorkItem(ctx context.Context, w *WorkItem) error

	RecordTransition(ctx context.Context, h *TransitionHistory) error
	GetTransitionHistory(ctx context.Context, workItemID string) ([]*TransitionHistory, error)
}

type ApprovalStore interface {
	RecordApproval(ctx context.Context, a *Approval) error
	ListApprovals(ctx context.Context, workItemID, stepID string) ([]*Approval, error)
}

// Store combines all storage interfaces.
type Store interface {
	TemplateStore
	InstanceStore
	WorkItemStore
	ApprovalStore
	Ping(ctx context.Context) error
	io.Closer
}
```

- [ ] **Step 3: Commit** — `feat(domain): add error sentinels and Store port interfaces`

---

### Task 5: CEL guard evaluator

**Files:** `internal/domain/guard.go`

- [ ] **Step 1: Add cel-go dependency**

```bash
go get github.com/google/cel-go
go mod tidy
```

- [ ] **Step 2: Create `internal/domain/guard.go`**

Defines `GuardContext` (fields: `Item`, `Actor`, `Approval` — matching the spec's CEL variable names), `EvaluateGuard(expression string, ctx GuardContext) error`, and `ValidateGuard(expression string) error`.

CEL env uses three dynamic-map variables: `item`, `actor`, `approval`. Struct-to-map conversion uses a JSON round-trip.

```go
// SPDX-License-Identifier: GPL-2.0-only
package domain

import (
	"encoding/json"
	"fmt"

	"github.com/google/cel-go/cel"
)

type GuardContext struct {
	Item     GuardItem     `json:"item"`
	Actor    GuardActor    `json:"actor"`
	Approval GuardApproval `json:"approval"`
}

type GuardItem struct {
	Title    string         `json:"title"`
	Priority string         `json:"priority"`
	Fields   map[string]any `json:"fields"`
	Step     string         `json:"step"`
}

type GuardActor struct {
	RoleID  string `json:"role_id"`
	AgentID string `json:"agent_id"`
}

type GuardApproval struct {
	Count      int `json:"count"`
	Rejections int `json:"rejections"`
}

func celEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable("item", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("actor", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("approval", cel.MapType(cel.StringType, cel.DynType)),
	)
}

// EvaluateGuard returns nil if expression is empty or evaluates true.
// Returns ErrGuardDenied for false, ErrInvalidGuard for compile/eval errors.
func EvaluateGuard(expression string, ctx GuardContext) error {
	if expression == "" {
		return nil
	}
	env, err := celEnv()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidGuard, err)
	}
	ast, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("%w: %v", ErrInvalidGuard, issues.Err())
	}
	prg, err := env.Program(ast)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidGuard, err)
	}
	data, _ := json.Marshal(ctx)
	var vars map[string]any
	json.Unmarshal(data, &vars) //nolint:errcheck
	out, _, err := prg.Eval(vars)
	if err != nil {
		return fmt.Errorf("%w: eval: %v", ErrInvalidGuard, err)
	}
	if result, ok := out.Value().(bool); !ok || !result {
		return ErrGuardDenied
	}
	return nil
}

// ValidateGuard compiles the expression and returns ErrInvalidGuard if it fails.
func ValidateGuard(expression string) error {
	if expression == "" {
		return nil
	}
	env, err := celEnv()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidGuard, err)
	}
	_, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("%w: %v", ErrInvalidGuard, issues.Err())
	}
	return nil
}
```

- [ ] **Step 3: Commit** — `feat(domain): add CEL guard evaluator`

---

## Chunk 3: SQLite Store

### Task 6: Store foundation and migrations

**Files:** `internal/infra/sqlite/store.go`, `internal/infra/sqlite/errors.go`, `internal/infra/sqlite/migrations/001_init.sql`, `internal/infra/open.go`

- [ ] **Step 1: Add dependencies**

```bash
go get modernc.org/sqlite
go get github.com/pressly/goose/v3
go mod tidy
```

- [ ] **Step 2: Create `internal/infra/sqlite/store.go`** — follow `hive/lead/internal/infra/sqlite/store.go` exactly (embedded migrations, WAL mode, foreign keys, goose Up).

- [ ] **Step 3: Create `internal/infra/sqlite/errors.go`** — `isUniqueViolation(err) bool` and `isFKViolation(err) bool` checking `err.Error()` for constraint strings.

- [ ] **Step 4: Create `internal/infra/sqlite/migrations/001_init.sql`**

```sql
-- +goose Up

CREATE TABLE workflow_templates (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    version     INTEGER NOT NULL DEFAULT 1,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE steps (
    id           TEXT PRIMARY KEY,
    template_id  TEXT NOT NULL REFERENCES workflow_templates(id) ON DELETE CASCADE,
    key          TEXT NOT NULL,
    name         TEXT NOT NULL,
    type         TEXT NOT NULL CHECK (type IN ('task', 'gate')),
    position     INTEGER NOT NULL DEFAULT 0,
    approval_mode              TEXT CHECK (approval_mode IN ('any', 'unanimous')),
    approval_required          INTEGER,
    approval_approver_role_id  TEXT,
    approval_rejection_step_id TEXT,
    UNIQUE (template_id, key)
);

CREATE TABLE transitions (
    id               TEXT PRIMARY KEY,
    template_id      TEXT NOT NULL REFERENCES workflow_templates(id) ON DELETE CASCADE,
    key              TEXT NOT NULL,
    name             TEXT NOT NULL,
    from_step_id     TEXT NOT NULL REFERENCES steps(id),
    to_step_id       TEXT NOT NULL REFERENCES steps(id),
    guard            TEXT NOT NULL DEFAULT '',
    required_role_id TEXT NOT NULL DEFAULT '',
    UNIQUE (template_id, key)
);

CREATE TABLE role_mappings (
    id              TEXT PRIMARY KEY,
    template_id     TEXT NOT NULL REFERENCES workflow_templates(id) ON DELETE CASCADE,
    step_id         TEXT NOT NULL REFERENCES steps(id) ON DELETE CASCADE,
    role_id         TEXT NOT NULL,
    allowed_actions TEXT NOT NULL DEFAULT '[]'
);

CREATE TABLE integration_hooks (
    id            TEXT PRIMARY KEY,
    template_id   TEXT NOT NULL REFERENCES workflow_templates(id) ON DELETE CASCADE,
    transition_id TEXT NOT NULL REFERENCES transitions(id) ON DELETE CASCADE,
    event         TEXT NOT NULL DEFAULT 'on_transition',
    adapter_type  TEXT NOT NULL,
    action        TEXT NOT NULL,
    config        TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE workflow_instances (
    id               TEXT PRIMARY KEY,
    template_id      TEXT NOT NULL REFERENCES workflow_templates(id),
    template_version INTEGER NOT NULL,
    team_id          TEXT NOT NULL,
    name             TEXT NOT NULL,
    status           TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'paused', 'completed', 'archived')),
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE integration_configs (
    id           TEXT PRIMARY KEY,
    instance_id  TEXT NOT NULL REFERENCES workflow_instances(id) ON DELETE CASCADE,
    adapter_type TEXT NOT NULL,
    config       TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE work_items (
    id                TEXT PRIMARY KEY,
    instance_id       TEXT NOT NULL REFERENCES workflow_instances(id),
    title             TEXT NOT NULL,
    description       TEXT NOT NULL DEFAULT '',
    current_step_id   TEXT NOT NULL REFERENCES steps(id),
    assigned_agent_id TEXT NOT NULL DEFAULT '',
    priority          TEXT NOT NULL DEFAULT 'normal'
        CHECK (priority IN ('critical', 'high', 'normal', 'low')),
    fields            TEXT NOT NULL DEFAULT '{}',
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE external_links (
    id           TEXT PRIMARY KEY,
    work_item_id TEXT NOT NULL REFERENCES work_items(id) ON DELETE CASCADE,
    service_type TEXT NOT NULL,
    adapter      TEXT NOT NULL,
    external_id  TEXT NOT NULL,
    url          TEXT NOT NULL DEFAULT ''
);

CREATE TABLE transition_history (
    id            TEXT PRIMARY KEY,
    work_item_id  TEXT NOT NULL REFERENCES work_items(id),
    from_step_id  TEXT NOT NULL REFERENCES steps(id),
    to_step_id    TEXT NOT NULL REFERENCES steps(id),
    transition_id TEXT NOT NULL REFERENCES transitions(id),
    triggered_by  TEXT NOT NULL,
    reason        TEXT NOT NULL DEFAULT '',
    timestamp     DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE approvals (
    id           TEXT PRIMARY KEY,
    work_item_id TEXT NOT NULL REFERENCES work_items(id),
    step_id      TEXT NOT NULL REFERENCES steps(id),
    agent_id     TEXT NOT NULL,
    decision     TEXT NOT NULL CHECK (decision IN ('approved', 'rejected')),
    comment      TEXT NOT NULL DEFAULT '',
    timestamp    DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- +goose Down

DROP TABLE approvals;
DROP TABLE transition_history;
DROP TABLE external_links;
DROP TABLE work_items;
DROP TABLE integration_configs;
DROP TABLE workflow_instances;
DROP TABLE integration_hooks;
DROP TABLE role_mappings;
DROP TABLE transitions;
DROP TABLE steps;
DROP TABLE workflow_templates;
```

- [ ] **Step 5: Create `internal/infra/open.go`** — delegates to `sqlite.Open`. Postgres is Phase 2.

```go
// SPDX-License-Identifier: GPL-2.0-only
package infra

import (
	"fmt"
	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/infra/sqlite"
)

func Open(dsn string) (domain.Store, error) {
	s, err := sqlite.Open(dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite store: %w", err)
	}
	return s, nil
}
```

- [ ] **Step 6: Verify** — `go build ./...` exits 0.

- [ ] **Step 7: Commit** — `feat(infra): add SQLite store foundation with Goose migrations`

---

### Task 7: SQLite store — template operations

**Files:** `internal/infra/sqlite/templates.go`

- [ ] **Step 1: Implement `TemplateStore` interface**

Follow the Hive SQLite pattern (`hive/lead/internal/infra/sqlite/teams.go` and `roles.go` for reference). Key details:

- `CreateTemplate`: single transaction — insert template row, then loop inserting steps, transitions, role_mappings, integration_hooks. Gate step approval fields go in the `steps` row nullable columns. `allowed_actions` stored as JSON (`json.Marshal([]string{...})`).
- `GetTemplate`: separate queries for each sub-entity, then assemble. No joins.
- `DeleteTemplate`: check `workflow_instances` count > 0 → return `ErrHasDependencies`.
- `UpdateTemplate`: update template row `name`, `description`, `version`, `updated_at`. Steps/transitions/hooks are replaced (delete all + re-insert in transaction).
- `ListTemplates`: returns header rows only (no sub-entities for list).

- [ ] **Step 2: Commit** — `feat(infra): implement SQLite TemplateStore`

---

### Task 8: SQLite store — instances, work items, approvals

**Files:** `internal/infra/sqlite/instances.go`, `internal/infra/sqlite/workitems.go`, `internal/infra/sqlite/approvals.go`

- [ ] **Step 1: Implement `InstanceStore` in `instances.go`**

- `CreateInstance`: insert instance row + `IntegrationConfigs` in transaction.
- `ListInstances(teamID)`: if `teamID == ""` return all; otherwise filter by `team_id`.
- `UpdateInstance`: update `name`, `status`, `updated_at`.

- [ ] **Step 2: Implement `WorkItemStore` in `workitems.go`**

- `ListWorkItems`: `instanceID` always used in WHERE; `stepID`, `agentID`, `priority` are optional — build parameterized query with conditional appends (use a `[]any` args slice, never string concatenation).
- `GetWorkItem`: fetch work item + fetch `external_links` separately.
- `UpdateWorkItem`: update all mutable fields.
- `RecordTransition`: INSERT only — transition history is append-only.

- [ ] **Step 3: Implement `ApprovalStore` in `approvals.go`**

- `RecordApproval`: INSERT.
- `ListApprovals(workItemID, stepID)`: filter by `work_item_id`; if `stepID != ""` also filter by `step_id`.

- [ ] **Step 4: Write minimal store test** in `internal/infra/sqlite/store_test.go`:

```go
func TestStoreOpen(t *testing.T) {
    s, err := sqlite.Open("")
    require.NoError(t, err)
    defer s.Close()
    require.NoError(t, s.Ping(context.Background()))
}
```

- [ ] **Step 5: Run** — `go test ./internal/infra/sqlite/...` passes.

- [ ] **Step 6: Commit** — `feat(infra): implement SQLite InstanceStore, WorkItemStore, ApprovalStore`

---

## Chunk 4: HTTP Server and REST API

### Task 9: Health handler, ID generator, HTTP server

**Files:** `internal/daemon/health.go`, `internal/daemon/id.go`, `internal/daemon/server.go`

- [ ] **Step 1: Create `internal/daemon/health.go`** — follow `hive/lead/internal/daemon/health.go` exactly (CheckResult, HealthService with periodic checks, HandleHealth returning 200/218/503).

Add `HandleUIHealth()` for Pylon service discovery:

```go
func HandleUIHealth() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(map[string]string{
            "name":  "flow",
            "label": "Flow",
            "route": "/flow",
        }) //nolint:errcheck
    }
}
```

- [ ] **Step 2: Add uuid dependency and create `internal/daemon/id.go`**

```bash
go get github.com/google/uuid && go mod tidy
```

```go
// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"fmt"
	"strings"
	"github.com/google/uuid"
)

// NewID returns a prefixed short ID: e.g. "tpl_a1b2c3d4".
func NewID(prefix string) string {
	id := strings.ReplaceAll(uuid.New().String(), "-", "")
	return fmt.Sprintf("%s_%s", prefix, id[:8])
}
```

- [ ] **Step 3: Create `internal/daemon/server.go`** — follow `hive/lead/internal/daemon/server.go` exactly. `ServerConfig` holds `Bind`, `Port`, `PassportURL`, `Health *HealthService`, `Store domain.Store`. Register:

```go
mux.HandleFunc("GET /v1/health", HandleHealth(cfg.Health))
mux.HandleFunc("GET /ui/health", HandleUIHealth())
// route groups registered in Tasks 10-11
// MCP handler registered in Task 15
```

Passport auth middleware: same conditional pattern as Hive.

- [ ] **Step 4: Commit** — `feat(daemon): add health handler, ID generator, HTTP server`

---

### Task 10: REST API — templates and instances

**Files:** `internal/daemon/rest_types.go`, `internal/daemon/rest_huma.go`

- [ ] **Step 1: Create `internal/daemon/rest_types.go`** with all Huma input/output structs.

Follow Hive's naming convention (`{Entity}Output`, `Create{Entity}Input`, etc.). Define:

- `IDPathInput { ID string \`path:"id"\` }`
- `templateResponse`, `stepResponse`, `transitionResponse`, `TemplateOutput`, `TemplateListOutput`
- `CreateTemplateInput` (name, description)
- `instanceResponse`, `InstanceOutput`, `InstanceListOutput`
- `CreateInstanceInput` (template_id, team_id, name)
- `PatchInstanceInput` (name, status)

- [ ] **Step 2: Create `internal/daemon/rest_huma.go`** with `mapDomainErr` and route registration functions. Follow Hive's `registerTeamRoutes` pattern exactly.

**`registerTemplateRoutes(api, store)`** — 5 routes:

| Method | Path | OperationID |
|--------|------|-------------|
| GET | `/v1/templates` | `list-templates` |
| POST | `/v1/templates` | `create-template` (201) |
| GET | `/v1/templates/{id}` | `get-template` |
| PATCH | `/v1/templates/{id}` | `update-template` |
| DELETE | `/v1/templates/{id}` | `delete-template` (204) |

**`registerInstanceRoutes(api, store)`** — 4 routes:

| Method | Path | OperationID |
|--------|------|-------------|
| GET | `/v1/instances` | `list-instances` (query: `?team_id=`) |
| POST | `/v1/instances` | `create-instance` (201, snapshot template version) |
| GET | `/v1/instances/{id}` | `get-instance` |
| PATCH | `/v1/instances/{id}` | `update-instance` |

- [ ] **Step 3: Register in server.go** — `registerTemplateRoutes(api, cfg.Store)`, `registerInstanceRoutes(api, cfg.Store)`.

- [ ] **Step 4: Commit** — `feat(daemon): add REST handlers for templates and instances`

---

### Task 11: REST API — work items, transitions, approvals

**Files:** Modify `internal/daemon/rest_types.go`, `internal/daemon/rest_huma.go`

- [ ] **Step 1: Add work item types to `rest_types.go`**

- `workItemResponse`, `WorkItemOutput`, `WorkItemListOutput`, `WorkItemDetailOutput` (includes history)
- `CreateWorkItemInput` (path `instance_id`, body: title, description, assigned_agent_id, priority)
- `PatchWorkItemInput` (title, description, assigned_agent_id, fields)
- `TransitionWorkItemInput` (transition_id, actor_agent_id, actor_role_id, reason)
- `ApproveWorkItemInput` / `RejectWorkItemInput` (agent_id, comment)

- [ ] **Step 2: Add route registration to `rest_huma.go`**

**`registerWorkItemRoutes(api, store)`** — 5 routes:

| Method | Path | OperationID | Notes |
|--------|------|-------------|-------|
| POST | `/v1/instances/{id}/items` | `create-work-item` (201) | initial `current_step_id` = first step (lowest position) |
| GET | `/v1/instances/{id}/items` | `list-work-items` | query: `?step_id=&agent_id=&priority=` |
| GET | `/v1/items/{id}` | `get-work-item` | |
| PATCH | `/v1/items/{id}` | `update-work-item` | |
| GET | `/v1/items/{id}/history` | `get-work-item-history` | |

**`registerTransitionRoutes(api, store)`** — 1 route:

`POST /v1/items/{id}/transition` (`transition-work-item`):
1. Load work item, verify `CurrentStepID == transition.FromStepID` → 422 on mismatch (`ErrInvalidTransition`).
2. Build `domain.GuardContext` from request fields.
3. `domain.EvaluateGuard(transition.Guard, ctx)` → 422 on `ErrGuardDenied`.
4. `store.UpdateWorkItem` with new `CurrentStepID = transition.ToStepID`.
5. `store.RecordTransition` — append history entry.
6. Return updated work item.

**`registerApprovalRoutes(api, store)`** — 3 routes:

| Method | Path | OperationID | Notes |
|--------|------|-------------|-------|
| POST | `/v1/items/{id}/approve` | `approve-work-item` | verify gate step; insert approval; auto-advance if threshold met |
| POST | `/v1/items/{id}/reject` | `reject-work-item` | verify gate step; insert rejection; advance to `rejection_step_id` if set |
| GET | `/v1/items/{id}/approvals` | `list-approvals` | |

Auto-advance logic for `approve`: load all approvals for the current step, count decisions. For `mode=any`: if `count(approved) >= required_approvers`, find the outgoing transition from the current gate step (the one NOT going to the rejection step), update work item `current_step_id`.

- [ ] **Step 3: Register in server.go** — add `registerWorkItemRoutes`, `registerTransitionRoutes`, `registerApprovalRoutes`.

- [ ] **Step 4: Commit** — `feat(daemon): add REST handlers for work items, transitions, approvals`

---

### Task 12: Daemon subcommand

**Files:** `cmd/daemon/daemon.go`

- [ ] **Step 1: Create `cmd/daemon/daemon.go`** — follow `hive/lead/cmd/daemon/daemon.go` exactly. Flags: `--bind`, `--port`, `--db`, `--passport-url`. In `run()`: call `infra.Open(db)`, create `HealthService`, register DB ping as boot check, create server, handle SIGINT/SIGTERM with 15s graceful shutdown.

- [ ] **Step 2: Register in `cmd/root.go`** — add `daemonCmd.NewCmd()`.

- [ ] **Step 3: Verify**

```bash
go mod tidy && go build -o build/flow .
./build/flow daemon --port 17201 --db /tmp/flow-test.db &
sleep 1
curl -s http://127.0.0.1:17201/v1/health | jq .
curl -s http://127.0.0.1:17201/ui/health | jq .
kill %1
rm /tmp/flow-test.db
```

Expected:
- `/v1/health`: `{"status":"healthy","checks":[{"name":"db","severity":"ok"}]}`
- `/ui/health`: `{"name":"flow","label":"Flow","route":"/flow"}`

- [ ] **Step 4: Commit** — `feat(cmd): add flow daemon subcommand`

---

## Chunk 5: MCP Bridge

### Task 13: MCP tool handlers

**Files:** `internal/daemon/mcp_server.go`, `internal/daemon/mcp_tools.go`

- [ ] **Step 1: Add mcp-go dependency** — `go get github.com/mark3labs/mcp-go && go mod tidy`

- [ ] **Step 2: Create `internal/daemon/mcp_server.go`** — follow `hive/lead/internal/daemon/mcp_server.go`. `MCPDeps` holds `Store domain.Store`. `NewMCPHandler` creates a `StreamableHTTPServer`. Mount at `/mcp` in `server.go`.

- [ ] **Step 3: Create `internal/daemon/mcp_tools.go`** with `jsonResult` helper and all 12 tools:

| Tool | Parameters | Store call |
|------|-----------|------------|
| `list_templates` | — | `ListTemplates` |
| `get_template` | `id` (req) | `GetTemplate` |
| `create_instance` | `template_id`, `team_id`, `name` (all req) | `CreateInstance` |
| `list_instances` | `team_id` | `ListInstances` |
| `create_work_item` | `instance_id`, `title` (req); `description`, `assigned_agent_id`, `priority` | `CreateWorkItem` |
| `list_work_items` | `instance_id` (req); `step_id`, `agent_id`, `priority` | `ListWorkItems` |
| `get_work_item` | `id` (req) | `GetWorkItem` + `GetTransitionHistory` |
| `transition_work_item` | `id`, `transition_id`, `actor_agent_id`, `actor_role_id` (all req); `reason` | guard eval + `UpdateWorkItem` + `RecordTransition` |
| `approve_work_item` | `id`, `agent_id` (req); `comment` | `RecordApproval` + auto-advance |
| `reject_work_item` | `id`, `agent_id` (req); `comment` | `RecordApproval` + advance to rejection step |
| `assign_work_item` | `id`, `agent_id` (both req) | `GetWorkItem` + `UpdateWorkItem` (agent field only) |
| `get_instance_status` | `id` (req) | `GetInstance` + `ListWorkItems` grouped by step |

Use `mcp.WithString`, `mcp.Required()`, `mcp.Description()` for all parameters.

- [ ] **Step 4: Commit** — `feat(daemon): add MCP tool handlers with full REST API parity`

---

### Task 14: MCP bridge subcommand

**Files:** `cmd/mcpbridge/mcp_bridge.go`

- [ ] **Step 1: Create `cmd/mcpbridge/mcp_bridge.go`** — follow `hive/lead/cmd/mcpbridge/mcp_bridge.go` exactly. Flags: `--agent-id` (required), `--host`, `--port 17200`. Sets `X-Agent-Id` header; tracks `Mcp-Session-Id` session header across requests.

- [ ] **Step 2: Register in `cmd/root.go`** — add `mcpBridgeCmd.NewCmd()`.

- [ ] **Step 3: Commit** — `feat(cmd): add MCP bridge subcommand (stdio-to-HTTP)`

---

## Chunk 6: Admin Command and SDLC Seed

### Task 15: Template JSON importer

**Files:** `internal/transfer/import.go`

This package converts the portable JSON template format (schema_version 0.1.0) into domain entities.

- [ ] **Step 1: Create `internal/transfer/import.go`**

Define `TemplateFile`, `StepFile`, `ApprovalFile`, `TransitionFile`, `RoleMappingFile`, `HookFile` structs mirroring the JSON schema in `docs/workflow-schema-draft.json`.

Define `RoleIndex map[string]string` (semantic name → Hive role ID; in Phase 1 pass `nil` — role names used as-is).

`ImportTemplate(ctx, store, path string, roles RoleIndex) (*domain.WorkflowTemplate, error)` — reads JSON file, calls `ImportTemplateFromFile`.

`ImportTemplateFromFile(ctx, store, *TemplateFile, roles RoleIndex) (*domain.WorkflowTemplate, error)`:
1. Generate template UUID.
2. Assign UUIDs to steps (key → UUID map) and transitions (key → UUID map).
3. Resolve role names: `roles[name]` if present, else use name as-is.
4. Resolve `rejection_step` key → step UUID; error if not found.
5. Validate all guard expressions via `domain.ValidateGuard`.
6. Resolve transition `from`/`to` keys → step UUIDs; error if not found.
7. Resolve hook `transition` key → transition UUID.
8. Build domain structs and call `store.CreateTemplate`.
9. Return `store.GetTemplate` result.

- [ ] **Step 2: Commit** — `feat(transfer): add JSON template importer`

---

### Task 16: Admin subcommand

**Files:** `cmd/admin/admin.go`

- [ ] **Step 1: Create `cmd/admin/admin.go`**

```go
// SPDX-License-Identifier: GPL-2.0-only
package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/Work-Fort/Flow/internal/infra"
	"github.com/Work-Fort/Flow/internal/transfer"
)

func NewCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "admin", Short: "Admin commands"}
	cmd.AddCommand(newSeedCmd())
	return cmd
}

func newSeedCmd() *cobra.Command {
	var db, file string
	cmd := &cobra.Command{
		Use:   "seed",
		Short: "Load a workflow template from a JSON file",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := infra.Open(db)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer store.Close()
			t, err := transfer.ImportTemplate(context.Background(), store, file, nil)
			if err != nil {
				return fmt.Errorf("import template: %w", err)
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(t)
		},
	}
	cmd.Flags().StringVar(&db, "db", "", "Database path (required)")
	cmd.Flags().StringVar(&file, "file", "", "Path to workflow JSON template (required)")
	cmd.MarkFlagRequired("db")
	cmd.MarkFlagRequired("file")
	return cmd
}
```

- [ ] **Step 2: Register in `cmd/root.go`** — add `adminCmd.NewCmd()`.

- [ ] **Step 3: Test seed with SDLC template**

```bash
go build -o build/flow .
./build/flow admin seed \
  --db /tmp/flow-seed-test.db \
  --file /home/kazw/Work/WorkFort/flow/lead/docs/examples/sdlc-template.json \
  | jq '{name: .name, steps: (.steps | length), transitions: (.transitions | length)}'
```

Expected: `{"name":"Software Development Lifecycle","steps":10,"transitions":15}`

- [ ] **Step 4: Commit** — `feat(cmd): add admin seed command`

---

## Chunk 7: Tests and Final Verification

### Task 17: Unit tests

**Files:** `internal/domain/guard_test.go`, `internal/infra/sqlite/store_test.go`

- [ ] **Step 1: Create `internal/domain/guard_test.go`**

```go
func TestEvaluateGuard_Empty(t *testing.T)          // returns nil
func TestEvaluateGuard_True(t *testing.T)           // expression `item.priority == "high"`, priority="high" → nil
func TestEvaluateGuard_False(t *testing.T)          // same expression, priority="low" → ErrGuardDenied
func TestEvaluateGuard_InvalidSyntax(t *testing.T)  // `item.priority ==` → ErrInvalidGuard
func TestValidateGuard_Valid(t *testing.T)          // nil
func TestValidateGuard_Invalid(t *testing.T)        // ErrInvalidGuard
```

- [ ] **Step 2: Expand `internal/infra/sqlite/store_test.go`**

```go
func TestStoreOpen(t *testing.T)              // Open + Ping + Close
func TestTemplateRoundTrip(t *testing.T)      // CreateTemplate with 2 steps + 1 transition → GetTemplate → ListTemplates → DeleteTemplate
func TestWorkItemTransitionFlow(t *testing.T) // Create template→instance→work item → RecordTransition + UpdateWorkItem → GetTransitionHistory
func TestApprovalFlow(t *testing.T)           // Create gate step work item → RecordApproval → ListApprovals
```

- [ ] **Step 3: Run all tests**

```bash
go test ./... -v
```

Expected: all pass.

- [ ] **Step 4: Commit** — `test: add unit tests for domain guard and SQLite store`

---

### Task 18: End-to-end smoke test

- [ ] **Step 1: Full build**

```bash
mise run build
```

- [ ] **Step 2: Start daemon and verify health**

```bash
./build/flow daemon --port 17299 --db /tmp/flow-smoke.db &
sleep 1
curl -s http://127.0.0.1:17299/v1/health | jq .
curl -s http://127.0.0.1:17299/ui/health | jq .
```

- [ ] **Step 3: Seed SDLC template and verify via REST**

```bash
./build/flow admin seed --db /tmp/flow-smoke.db \
  --file /home/kazw/Work/WorkFort/flow/lead/docs/examples/sdlc-template.json > /dev/null

curl -s http://127.0.0.1:17299/v1/templates | jq '.[0].name'
```

Expected: `"Software Development Lifecycle"`

- [ ] **Step 4: Create instance and work item**

```bash
TMPL=$(curl -s http://127.0.0.1:17299/v1/templates | jq -r '.[0].id')
INST=$(curl -s -XPOST http://127.0.0.1:17299/v1/instances \
  -H "Content-Type: application/json" \
  -d "{\"template_id\":\"$TMPL\",\"team_id\":\"t1\",\"name\":\"Test\"}" | jq -r '.id')
curl -s -XPOST "http://127.0.0.1:17299/v1/instances/$INST/items" \
  -H "Content-Type: application/json" \
  -d '{"title":"Feature A","priority":"high"}' | jq '{id:.id, step:.current_step_id}'
```

- [ ] **Step 5: Verify MCP tools list**

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' \
  | ./build/flow mcp-bridge --agent-id test --port 17299
```

Expected: JSON with `"tools"` array of 12 items.

- [ ] **Step 6: Teardown**

```bash
kill %1
rm /tmp/flow-smoke.db
```

- [ ] **Step 7: Final test run**

```bash
mise run test
```

Expected: exit 0, all pass.

- [ ] **Step 8: Commit** — `feat: complete Flow Phase 1 core engine`

---

## QA Testing Checklist

After implementation, QA verifies the following against a running daemon:

**Health**
- [ ] `GET /v1/health` → 200, `status: healthy`
- [ ] `GET /ui/health` → 200, `{"name":"flow","label":"Flow","route":"/flow"}`

**Templates**
- [ ] Create/list/get/update/delete template
- [ ] Delete with active instance → 409

**Instances**
- [ ] Create instance from template (version snapshot)
- [ ] List filtered by `?team_id=`
- [ ] Patch status to `paused`, `archived`

**Work Items and Transitions**
- [ ] Create work item — `current_step_id` is first step (lowest position)
- [ ] Transition with valid transition ID — step advances, history recorded
- [ ] Transition with failing CEL guard → 422
- [ ] Transition with wrong `from_step_id` → 422

**Approvals**
- [ ] Approve at gate step — approval recorded
- [ ] Approve/reject at non-gate step → 422
- [ ] After N approvals in `any` mode — work item auto-advances

**Admin Seed**
- [ ] `flow admin seed` creates SDLC template with 10 steps, 15 transitions
- [ ] Running seed again on same DB does not silently duplicate

**MCP Bridge**
- [ ] `tools/list` returns 12 tools
- [ ] `list_templates` returns seeded template
- [ ] `transition_work_item` tool advances a work item

**Build**
- [ ] `mise run build` exits 0
- [ ] `mise run test` exits 0, all pass
- [ ] `./build/flow --version` → `flow version dev`
