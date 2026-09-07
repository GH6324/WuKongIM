# message/webhook AGENTS

This scenario owns black-box webhook coverage for `cmd/wukongim`.

## Purpose

Prove single-node cluster post-commit notifications and three-node synchronous
msg.before_send admission through real WKProto, Product HTTP, committed history,
and an external HTTP endpoint. Cover payload mutation, business rejection,
independent timeout/error policies, transient sends, and one callback per ingress
attempt across authority forwarding.

## Run

```bash
GOWORK=off go test -tags=e2e ./test/e2e/message/webhook -count=1 -timeout 2m -p=1
```

## Rules

- Keep assertions black-box through real `cmd/wukongim`, real WKProto SEND,
  and HTTP requests observed by the test webhook endpoint.
- Do not import `internal/app`, `internal/usecase`, or storage internals.
- Keep webhook waits bounded and include node diagnostics plus captured webhook
  requests on failure.

- Gateway token authentication is explicitly disabled in this scenario to isolate
  callback behavior; these tests do not establish an authentication claim.
