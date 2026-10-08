#!/bin/bash
set -euo pipefail
tag=${1:?usage: scripts/package.sh <tag>}
if [[ $tag != v* ]]; then
	echo "package: tag must start with v" >&2
	exit 1
fi
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT

mkdir -p "$stage/.claude-plugin" "$stage/bin" "$stage/hooks" "$stage/skills/budget"
cp "$root/.claude-plugin/marketplace.json" "$stage/.claude-plugin/"
jq --arg version "${tag#v}" '.version = $version' "$root/.claude-plugin/plugin.json" >"$stage/.claude-plugin/plugin.json"
cp "$root/bin/ctx" "$stage/bin/"
for target in linux/amd64 linux/arm64 darwin/arm64; do
	(cd "$root" && CGO_ENABLED=0 GOOS=${target%/*} GOARCH=${target#*/} go build -trimpath -o "$stage/bin/ctx-${target%/*}-${target#*/}" ./cmd/ctx)
done
cp "$root/hooks/hooks.json" "$stage/hooks/"
cp "$root/skills/budget/SKILL.md" "$stage/skills/budget/"
cp "$root/install.sh" "$stage/"

mkdir -p "$root/dist"
rm -f "$root/dist/ctx-$tag.zip"
(cd "$stage" && zip -qr "$root/dist/ctx-$tag.zip" .)
echo "$root/dist/ctx-$tag.zip"
