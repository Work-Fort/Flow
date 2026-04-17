# Bug: NewID Truncates UUID to 8 Hex Characters

## Problem

`internal/daemon/id.go` lines 13-14:

```go
id := strings.ReplaceAll(uuid.New().String(), "-", "")
return fmt.Sprintf("%s_%s", prefix, id[:8])
```

This truncates a UUID to 8 hex characters (32 bits of entropy). Birthday
bound gives 50% collision probability at ~65,000 records. This is used for
template IDs, instance IDs, work item IDs, step IDs, and transition IDs.

## Fix

Use the full UUID:

```go
func NewID(prefix string) string {
    return fmt.Sprintf("%s_%s", prefix, uuid.New().String())
}
```

See `codex/src/architecture/go-service-patterns.md` for the standard pattern.

## Severity

High — will cause collisions in production use.
