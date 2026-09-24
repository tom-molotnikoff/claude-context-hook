# ctx

A Claude Code plugin that tells an agent when its context is filling up, so it can decide whether there is room for the next task in a batch. Start a batch with `/ctx:budget 75`.

## Install

On linux/amd64, with `gh` logged in:

```bash
tag=v0.1.0
gh release download "$tag" --repo tom-molotnikoff/claude-context-hook
mkdir -p ~/.local/share/ctx/"$tag"
unzip -q ctx-"$tag".zip -d ~/.local/share/ctx/"$tag"
~/.local/share/ctx/"$tag"/install.sh
```

`install.sh` adds the directory as the `ctx` marketplace, replacing any earlier one, and installs `ctx@ctx`. Start a new session to pick it up.

To upgrade or roll back, install another version the same way. To go back to a version that is already unpacked, run its `install.sh` again. Whichever was installed last is active in the next session.

## Release

Push a tag matching `v*`. The release workflow runs `go test ./...`, then builds `ctx-<tag>.zip` with `scripts/package.sh` and attaches it to a GitHub release.

## Development

```bash
go test ./...
go build -o bin/ctx-linux-amd64 ./cmd/ctx
claude --plugin-dir .
go run ./scripts/hookbench
```
