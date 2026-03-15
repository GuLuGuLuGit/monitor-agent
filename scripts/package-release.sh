#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
OUT_DIR="${1:-$REPO_DIR/../monitor-server/monitor-frontend/public/downloads/monitor-agent/latest}"
VERSION="${VERSION:-$(git -C "$REPO_DIR" rev-parse --short HEAD 2>/dev/null || date +%Y%m%d%H%M%S)}"
TARGETS=(
  "darwin/arm64"
  "darwin/amd64"
  "linux/arm64"
  "linux/amd64"
)

checksum_file() {
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1"
  else
    sha256sum "$1"
  fi
}

mkdir -p "$OUT_DIR"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

for target in "${TARGETS[@]}"; do
  GOOS="${target%/*}"
  GOARCH="${target#*/}"
  ASSET="monitor-agent-${GOOS}-${GOARCH}"
  BUILD_DIR="$TMP_DIR/$ASSET"
  mkdir -p "$BUILD_DIR"

  CGO_ENABLED=0
  if [[ "$GOOS" == "darwin" ]]; then
    CGO_ENABLED=1
  fi

  echo "building $ASSET"
  (
    cd "$REPO_DIR"
    GOOS="$GOOS" GOARCH="$GOARCH" CGO_ENABLED="$CGO_ENABLED" go build -o "$BUILD_DIR/monitor-agent" ./cmd/agent
  )
  cp "$REPO_DIR/config/config.example.yaml" "$BUILD_DIR/config.example.yaml"
  cat > "$BUILD_DIR/VERSION" <<TXT
$VERSION
TXT

  tar -C "$BUILD_DIR" -czf "$OUT_DIR/${ASSET}.tar.gz" .
done

(
  cd "$OUT_DIR"
  : > checksums.txt
  for asset in monitor-agent-*.tar.gz; do
    checksum_file "$asset" >> checksums.txt
  done
  cat > VERSION <<TXT
$VERSION
TXT
)

echo "release assets written to $OUT_DIR"
