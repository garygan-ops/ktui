# ktui Release Process

This document defines the release gate for stable ktui versions. Run releases from a clean `main` commit after CI has passed.

## Compatibility Contract

Starting with `v1.0.0`, these interfaces are stable within the v1 series:

- documented commands, flags, and their exit behavior;
- the JSON configuration format and profile migration behavior;
- JSON, CSV, and Markdown export fields;
- release asset names, checksums, and installer environment variables.

The packages under `internal/` are not a public Go API. Breaking a stable interface requires a major version or a documented compatibility path.

## Automated Gate

Run the same checks enforced by CI:

```sh
go mod verify
go test ./...
go vet ./...
go test -race ./...
sh -n install.sh
goreleaser check
goreleaser release --snapshot --clean
```

Confirm the snapshot contains Linux, macOS, and Windows archives for both amd64 and arm64, plus `checksums.txt`.

## RC Validation

Publish `v1.0.0-rc.1` before `v1.0.0`. Validate the following against real Komari instances with and without an API key:

- fresh setup and upgrade using an existing v0.5 configuration;
- TUI startup, list/detail/settings/about views, keyboard, mouse, resize, ASCII, and no-color modes;
- status, export, profile, completion, version, and update commands;
- Linux, macOS, and Windows installers and checksum verification;
- representative terminals on each supported operating system.

Keep the RC available for 7-14 days. Do not promote it while a crash, data loss, configuration corruption, installer failure, or undocumented CLI incompatibility remains open.

## Publish And Verify

1. Update the RC or stable release notes under `docs/releases/`.
2. Create and push an annotated `v*` tag from the verified commit.
3. Wait for the Release workflow to finish successfully.
4. Verify all six archives, `checksums.txt`, version metadata, and both installer paths.
5. For `v1.0.0`, use the RC commit when no fixes were needed; otherwise repeat the RC gate for the changed commit.
