# BHRouter (Brotherhood Router v0.0.1)

BHRouter is a single-binary Go tool for safely managing host/IP overrides in your OS hosts file.

- One codebase for CLI and UI
- Cross-platform targets via `GOOS` / `GOARCH`
- Managed block strategy to avoid damaging unmanaged host entries
- Automatic elevation attempt for CLI writes (sudo on macOS/Linux, run-as-admin on Windows)

## Build

```bash
go build -o dist/bhrouter ./cmd/bhrouter
```

Cross-compile examples:

```bash
GOOS=darwin GOARCH=arm64 go build -o dist/bhrouter-darwin-arm64 ./cmd/bhrouter
GOOS=linux GOARCH=amd64 go build -o dist/bhrouter-linux-amd64 ./cmd/bhrouter
GOOS=windows GOARCH=amd64 go build -o dist/bhrouter-windows-amd64.exe ./cmd/bhrouter
```

## CLI usage

```bash
bhrouter list
bhrouter set example.com 127.0.0.1
bhrouter remove example.com
bhrouter backup
bhrouter path
```

Use a custom hosts file for testing:

```bash
bhrouter --hosts /tmp/hosts.test set example.com 127.0.0.1
```

## UI usage

```bash
bhrouter ui --port 8787
```

Then open `http://127.0.0.1:8787`.

## Safety model

- BHRouter only edits entries inside a dedicated managed block:
  - `# >>> BHRouter managed block >>>`
  - `# <<< BHRouter managed block <<<`
- Unmanaged lines are preserved.
- A timestamped backup is written before changes:
  - `<hosts_path>.bhrouter.backup-YYYYMMDD-HHMMSS`
- Writes use temp-file replacement to reduce corruption risk.

## Notes

- On real system hosts files, write operations require admin/root permissions.
- UI runs as the current user. If permission is denied in UI mode, start BHRouter with elevated privileges.
