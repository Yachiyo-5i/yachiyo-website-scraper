#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_NAME="${APP_NAME:-scraper}"
ENTRYPOINT="${ENTRYPOINT:-./cmd/scraper}"
OUT_DIR="${OUT_DIR:-$ROOT_DIR/dist}"
VERSION_FILE="${VERSION_FILE:-$ROOT_DIR/version}"
GOOS_VALUE="${GOOS:-$(go env GOOS)}"
GOARCH_VALUE="${GOARCH:-$(go env GOARCH)}"
CGO_ENABLED_VALUE="${CGO_ENABLED:-0}"

if [[ ! -f "$VERSION_FILE" ]]; then
	echo "version file not found: $VERSION_FILE" >&2
	exit 1
fi

VERSION_VALUE="$(tr -d '\r' < "$VERSION_FILE" | sed -e '/^[[:space:]]*$/d' | head -n 1 | xargs)"
if [[ -z "$VERSION_VALUE" ]]; then
	echo "version file is empty: $VERSION_FILE" >&2
	exit 1
fi

mkdir -p "$OUT_DIR"

suffix=""
if [[ "$GOOS_VALUE" == "windows" ]]; then
	suffix=".exe"
fi

output="${OUT_DIR}/${APP_NAME}_${GOOS_VALUE}_${GOARCH_VALUE}${suffix}"

echo "Building binary:"
echo "  entrypoint: $ENTRYPOINT"
echo "  output:     $output"
echo "  version:    ${VERSION_VALUE}"
echo "  target:     ${GOOS_VALUE}/${GOARCH_VALUE}"
echo "  cgo:        ${CGO_ENABLED_VALUE}"

cd "$ROOT_DIR"

GOOS="$GOOS_VALUE" \
GOARCH="$GOARCH_VALUE" \
CGO_ENABLED="$CGO_ENABLED_VALUE" \
go build \
	-trimpath \
	-buildvcs=false \
	-o "$output" \
	"$ENTRYPOINT"

echo "Done: $output"
