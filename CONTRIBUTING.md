# Contributing

Thanks for taking a look. This plugin speaks to an undocumented vendor API, so
changes should be small and well-scoped.

## Ground rules

- Never commit cookies, `sessionid` values, auth files, or captured traffic that
  contains account identifiers.
- Keep the public plugin ABI and the three native protocol formats intact.
- Prefer adding a fixture-based test over testing against the live service.

## Development

```sh
cd go
go mod download
go test ./...
go vet ./...
```

The plugin is a cgo shared library, so tests and builds need a C compiler:

- Windows: MSYS2 `mingw64` toolchain (`C:\msys64\mingw64\bin`)
- macOS: Xcode command line tools
- Linux: `gcc`

## Changing an upstream endpoint

When Doubao changes a payload or event shape:

1. Capture only the structural fields (no cookies, no account ids).
2. Add the sanitized sample under `go/testdata/`.
3. Extend `parseDoubaoEvent`, `parseChatEvent` or the matching encoder.
4. Add a test that fails before the change and passes after it.

## Commit messages

Use imperative mood, one logical change per commit, e.g.:

```text
add Responses API reasoning summary events
fix QR flow expiry handling
```
