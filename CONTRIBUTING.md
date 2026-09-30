# Contributing

Thanks for helping improve the project.

## Development

The runtime is written entirely in Go. Go 1.21 or newer is required.

On Windows, run the complete local check and build with:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build.ps1
```

On any supported Go development platform, run:

```text
go test ./...
go vet ./...
```

## Design boundaries

- Keep user-configurable actions separate from engine-only lifecycle operations in `internal/core`.
- Keep bindings, fallback and app-profile matching in `internal/mapping`.
- Keep semantic shortcut recipes in `internal/desktop`; platform adapters translate only logical keys and true OS primitives.
- Keep controller lifecycle and gesture/session state in the focused files under `internal/engine`.
- Keep persisted-settings conversion in `cmd/couchpilot`; construct the engine with runtime `Options` only.
- Share the supported gesture catalog between validation and dispatch. All gestures use the same action lifecycle; test configuration through an engine step, not just resolver lookup.
- Retain held-input state until release succeeds. Cleanup retries must be bounded and persistent failures must propagate to the caller.
- Add OS implementations and build-tagged composition under `internal/platform`.
- Treat `config.json` as a versioned public contract; update validation and tests when changing it.
- Prefer small interfaces and data-driven bindings over a plugin framework.

## Pull requests

Please keep changes focused, add tests for behavior changes, and explain any new user-facing binding or configuration field. All tests and static checks must pass.
