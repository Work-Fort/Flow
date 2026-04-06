# Auth Config Fix

## Problem

Flow currently has separate token flags per external service:
- `--hive-token` (for calling Hive)
- The Sharkfin adapter design was about to add `--sharkfin-token`

This is wrong. WorkFort uses a single Passport identity per service. Flow
has one API key (`svc-flow`) that identifies it to ALL other services. Each
receiving service validates the token against Passport and sees "this is
svc-flow."

## Fix

1. Remove `--hive-token` from config and daemon flags
2. Use the existing `--passport-token` flag (already in config.go for
   auth middleware) as the single service identity token
3. Pass this token to all outbound adapters (Hive, Sharkfin, Combine)
4. Each adapter's constructor takes a URL + token, where the token is
   always the same Passport service token

Config should look like:
```
FLOW_PASSPORT_URL=http://passport:3000
FLOW_SERVICE_TOKEN=wf-svc_flow_xxxxx
FLOW_PYLON_URL=http://pylon:17300
# Service names default to "hive" and "sharkfin" — override only if needed:
# FLOW_PYLON_SERVICES_HIVE=hive
# FLOW_PYLON_SERVICES_SHARKFIN=sharkfin
```

Not:
```
FLOW_HIVE_TOKEN=wf-svc_flow_xxxxx
FLOW_SHARKFIN_TOKEN=wf-svc_flow_xxxxx  # same token duplicated
```

## Auth Model

- Passport issues one API key per service identity
- The key is universal — valid across all WorkFort services
- Receiving services validate via Passport's `/v1/verify-api-key`
- The response identifies the caller (svc-flow, type: service)
- No per-service scoping on keys
