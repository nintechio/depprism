#!/usr/bin/env bash
set -euo pipefail

release_version="${1:?usage: build-release.sh VERSION [OUTPUT_DIR]}"
release_output="${2:-dist}"
release_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
release_epoch="$(git -C "$release_root" log -1 --format=%ct)"
release_stage="$(mktemp -d)"
trap 'rm -rf "$release_stage"' EXIT

mkdir -p "$release_output"
release_output="$(cd "$release_output" && pwd)"

for release_target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
  release_goos="${release_target%/*}"
  release_goarch="${release_target#*/}"
  release_archive="depprism_${release_version}_${release_goos}_${release_goarch}"
  release_executable="depprism"
  release_extension="tar.gz"
  if [[ "$release_goos" == windows ]]; then
    release_executable="depprism.exe"
    release_extension="zip"
  fi

  mkdir -p "$release_stage/$release_archive"
  (
    cd "$release_root"
    CGO_ENABLED=0 GOOS="$release_goos" GOARCH="$release_goarch" go build \
      -trimpath -buildvcs=false -ldflags="-s -w -X main.version=$release_version" \
      -o "$release_stage/$release_archive/$release_executable" ./cmd/depprism
  )
  cp "$release_root/LICENSE" "$release_root/README.md" "$release_stage/$release_archive/"
  touch -d "@$release_epoch" "$release_stage/$release_archive" "$release_stage/$release_archive"/*

  rm -f "$release_output/$release_archive.$release_extension"
  if [[ "$release_extension" == zip ]]; then
    (cd "$release_stage" && zip -X -qr "$release_output/$release_archive.zip" "$release_archive")
  else
    tar --sort=name --mtime="@$release_epoch" --owner=0 --group=0 --numeric-owner \
      -C "$release_stage" -czf "$release_output/$release_archive.tar.gz" "$release_archive"
  fi
  rm -rf "$release_stage/$release_archive"
done

(
  cd "$release_output"
  sha256sum depprism_"$release_version"_* > checksums.txt
)
