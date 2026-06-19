# Verification

This repository includes a focused Go test suite plus a GitHub Actions workflow that runs on pushes and pull requests.

## Local Checks

The initial public branch was checked with:

```text
go test ./...
go vet ./...
go build -o /tmp/atl-smoke ./cmd/atl
ATL_HOME="$(mktemp -d)" /tmp/atl-smoke now --json
ATL_HOME="$(mktemp -d)" /tmp/atl-smoke serve-mcp
```

Covered behavior:

- RFC3339 timestamp output
- human duration formatting
- TTL parsing for `15s`, `30m`, `2h`, and `1d`
- stale checks before and after TTL expiry
- missing mark errors
- session status with no active session
- persisted sessions and marks
- valid JSONL audit events
- MCP tool discovery and `time_now`

## Public CI

The workflow at `.github/workflows/test.yml` runs:

```text
go test ./...
go vet ./...
```

That makes the tests visible on GitHub pull requests before merge.
