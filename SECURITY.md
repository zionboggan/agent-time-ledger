# Security

`agent-time-ledger` is local-first by design.

- No telemetry
- No cloud sync
- No prompt capture
- No arbitrary file reads through MCP tools
- No shell execution through MCP tools
- No network access from the CLI or MCP server

The default state directory is `~/.agent-time-ledger`. Set `ATL_HOME` to isolate state for testing.

Please report security issues privately rather than opening a public issue with exploit details.
