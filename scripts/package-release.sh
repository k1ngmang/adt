#!/usr/bin/env bash
set -euo pipefail

if [[ $# != 2 || ! $1 =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Usage: bash scripts/package-release.sh vMAJOR.MINOR.PATCH OUTPUT_DIR" >&2
  exit 2
fi

version=$1
output=$2
mkdir -p "$output"

staging=$(mktemp -d)
trap 'rm -r "$staging"' EXIT

cp LICENSE "$staging/LICENSE"

archives=()
for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64; do
  os=${target%/*}
  arch=${target#*/}
  name="adtgen_${version}_${os}_${arch}"
  executable=adtgen
  if [[ $os == windows ]]; then
    executable+=.exe
  fi

  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -trimpath -buildvcs=false -o "$staging/$executable" ./cmd/adtgen

  if [[ $os == windows ]]; then
    # Build a fresh archive before copying it to the output directory.
    zip -qj "$staging/$name.zip" "$staging/$executable" "$staging/LICENSE"
    archive="$name.zip"
  else
    archive="$name.tar.gz"
    tar -czf "$staging/$archive" -C "$staging" "$executable" LICENSE
  fi
  mv "$staging/$archive" "$output/$archive"
  archives+=("$archive")
done

(
  cd "$output"
  sha256sum "${archives[@]}" > SHA256SUMS
)
