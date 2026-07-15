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

- Keep device and operating-system APIs behind the interfaces in `internal/core`.
- Keep mapping decisions and pointer math in `internal/engine`.
- Add platform implementations under `internal/platform`.
- Treat `config.json` as a versioned public contract; update validation and tests when changing it.
- Prefer small interfaces and data-driven bindings over a plugin framework.

## Pull requests

Please keep changes focused, add tests for behavior changes, and explain any new user-facing binding or configuration field. All tests and static checks must pass.
