## What and why

<!-- The problem and its impact, then the change. Link the issue. -->

## Testing

<!-- Tests added, what they verify, and the server revision used. -->

- [ ] `go run ./cmd/verify -coverage` passes
- [ ] `govulncheck ./...` passes

## Compatibility

<!-- Existing callers, protocol versions, values and transaction behavior. -->

## Checklist

- [ ] One logical change per commit
- [ ] Normal, boundary and malformed-input tests; a regression test for a fix
- [ ] No unsafe, panics, uncertain-write retries or dependency without a decision
- [ ] RULES.md, CONFORMANCE.md, TODO.md, README.md and Go documentation updated where they change
