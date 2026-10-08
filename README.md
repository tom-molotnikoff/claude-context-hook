# ctx

A Claude Code plugin that tells an agent when its context is filling up, so it can decide whether there is room for the next task in a batch. Start a batch with `/ctx:budget 75`.

## Install

Releases carry binaries for Linux (amd64 and arm64) and Apple Silicon Macs. Pick the directory for your platform, then download, unzip and install:

```bash
tag=v1.0.0
dir=~/.local/share/ctx/"$tag"                        # Linux
dir=~/Library/Application\ Support/ctx/"$tag"        # macOS
curl -fsSLO https://github.com/tom-molotnikoff/claude-context-hook/releases/download/"$tag"/ctx-"$tag".zip
mkdir -p "$dir"
unzip -q ctx-"$tag".zip -d "$dir"
"$dir"/install.sh
```

`install.sh` adds the directory as the `ctx` marketplace, replacing any earlier one, and installs `ctx@ctx`. Start a new session to pick it up.

To upgrade or roll back, install another version the same way. To go back to a version that is already unpacked, run its `install.sh` again. Whichever was installed last is active in the next session.

State lives in `${XDG_STATE_HOME:-~/.local/state}/ctx` on Linux and `~/Library/Application Support/ctx` on macOS. `XDG_STATE_HOME` overrides the default on either.

## Release

Push a tag matching `v*`. The release workflow runs `go test ./...`, then builds `ctx-<tag>.zip` with `scripts/package.sh` and attaches it to a GitHub release.

## Development

```bash
go test ./...
go build -o bin/ctx-"$(go env GOOS)"-"$(go env GOARCH)" ./cmd/ctx
claude --plugin-dir .
go run ./scripts/hookbench
```

## License

MIT, see [LICENSE](LICENSE).
