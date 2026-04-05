# Sharkfin Chat Adapter Design

## Overview

Flow needs a Chat adapter to integrate with Sharkfin, enabling the workflow
engine to operate as a bot that posts state change notifications, receives
commands from agents, and provides status queries — all within Sharkfin
channels.

This is the next phase of Flow development after the Hive identity adapter
(plan 003).

## Prerequisites

All Sharkfin-side work is complete:
- Bot role with permissions (send_message, join_channel, create_channel, etc.)
- Service identities auto-assigned bot role
- Per-identity webhook registration (register_webhook, unregister_webhook MCP tools)
- Webhook recipient scope includes service identities in all channel messages
- Message metadata column (optional JSON sidecar on messages)
- Reply threading via thread_id in send_message

## Architecture

### Chat Port (domain layer)

The design doc defines a Chat port interface. This needs to be added to
`internal/domain/ports.go`:

```
PostMessage(channel, content, metadata) -> MessageID
PostBotUpdate(channel, workItemID, transition, metadata) -> MessageID
CreateChannel(name, members) -> ChannelID
```

### Sharkfin Adapter (infra layer)

`internal/infra/sharkfin/adapter.go` — implements the Chat port using
Sharkfin's REST API / MCP tools:

- `PostMessage` → calls Sharkfin's `send_message` with optional metadata
- `PostBotUpdate` → calls `send_message` with structured metadata
  (`{"event_type": "work_item_transitioned", "event_payload": {...}}`)
- `CreateChannel` → calls Sharkfin's `channel_create`

### Webhook Receiver

`internal/daemon/webhook_sharkfin.go` — HTTP handler for incoming Sharkfin
webhooks. Sharkfin POSTs to Flow when messages arrive in channels the bot
has joined.

Route: `POST /v1/webhooks/sharkfin`

The handler:
1. Validates the webhook signature (HMAC with shared secret)
2. Parses the payload (message body, metadata, sender, channel)
3. If metadata contains a Flow command (`event_type: "flow_command"`),
   dispatches to command handler
4. Otherwise ignores (bot doesn't need to act on every message)

### Command Handling

When an agent sends a structured command to Flow's bot (via metadata or
message body convention like `@flow transition #42 to review`):

1. Webhook delivers the message to Flow
2. Flow parses the command
3. Flow executes the action (transition, approve, status query, etc.)
4. Flow responds in the channel via `PostMessage` with the result

### Integration Hooks

The WorkflowService already fires integration hooks on transitions. The
Chat adapter is called when a hook's `adapter_type == "chat"`. Hook actions:

| Action | Adapter Call |
|--------|-------------|
| `post_message` | `PostMessage(channel, template, metadata)` |
| `assign_agent` | Not chat — handled by Identity adapter |
| `create_issue` | Not chat — handled by GitForge adapter |

The hook's `config` JSON contains the message template with `{{item.title}}`
style placeholders resolved at execution time.

### Bot Lifecycle

On daemon startup (if `--sharkfin-url` is configured):
1. Authenticate with Sharkfin using Passport service token
2. Register identity as `type: "service"` (auto-assigned bot role)
3. Register webhook callback URL (`POST /v1/webhooks/sharkfin`)
4. Join configured channels (or create them)

### Config

New flags:
- `--sharkfin-url` — Sharkfin daemon URL (e.g., `http://localhost:16000`)
- `--sharkfin-token` — Passport service token for Sharkfin auth
- `--sharkfin-webhook-url` — Flow's externally reachable URL for webhook
  callbacks (e.g., `http://flow:17200/v1/webhooks/sharkfin`)
- `--sharkfin-webhook-secret` — shared secret for HMAC signature

Adapter constructed only when `sharkfin-url` is non-empty (same pattern as
Hive adapter).

## Service Relationships

```
Flow daemon
  |
  +-- Sharkfin adapter (outbound: send messages, create channels)
  |     |
  |     +-- POST /mcp or REST to Sharkfin
  |
  +-- Webhook handler (inbound: receive messages from Sharkfin)
        |
        +-- POST /v1/webhooks/sharkfin (Sharkfin calls Flow)
```

## What This Enables

With the Sharkfin adapter, Flow becomes visible in chat:
- "Work item #42 moved to Code Review" appears in the team channel
- Agents can query `@flow status #42` and get a response
- The channel becomes a dashboard — every state change is a chat event
- The conversation is the audit log

## Testing

- Unit tests with a stub Chat provider (same pattern as stub identity)
- Test that nil Chat provider skips notifications (backwards compat)
- Test webhook signature validation
- Test command parsing and dispatch
- Integration test: post a message, receive webhook, verify round-trip
