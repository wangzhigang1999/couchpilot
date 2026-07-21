#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
architecture=${GOARCH:-$(go env GOARCH)}
deployment_target=${MACOSX_DEPLOYMENT_TARGET:-11.0}
cache_target=$(printf '%s' "$deployment_target" | tr -c '[:alnum:].-' '_')
output=${1:-"$root/bin/couchpilot-darwin-$architecture"}
app_bundle="$root/bin/CouchPilot.app"
stage=""
raw_stage=""
previous_bundle=""

cleanup() {
    if [ -n "$raw_stage" ] && [ -e "$raw_stage" ]; then
        rm -f -- "$raw_stage"
    fi
    if [ -n "$stage" ] && [ -d "$stage" ]; then
        rm -rf -- "$stage"
    fi
    if [ -n "$previous_bundle" ] && [ -d "$previous_bundle" ]; then
        if [ ! -e "$app_bundle" ]; then
            mv -- "$previous_bundle" "$app_bundle" || true
        else
            rm -rf -- "$previous_bundle"
        fi
    fi
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

export GOPATH="$root/.cache/gopath"
export GOMODCACHE="$GOPATH/pkg/mod"
export GOCACHE="$root/.cache/go-build/darwin-$architecture-macos-$cache_target"
export CGO_ENABLED=1
export GOOS=darwin
export GOARCH="$architecture"
export MACOSX_DEPLOYMENT_TARGET="$deployment_target"
export CGO_CFLAGS="${CGO_CFLAGS:-} -mmacosx-version-min=$deployment_target"
export CGO_LDFLAGS="${CGO_LDFLAGS:-} -mmacosx-version-min=$deployment_target"

mkdir -p "$(dirname -- "$output")"
mkdir -p "$root/bin"
cd "$root"
go mod download
go test ./...
go vet ./...

stage=$(mktemp -d "$root/bin/.couchpilot-build.XXXXXX")
staged_bundle="$stage/CouchPilot.app"
staged_executable="$staged_bundle/Contents/MacOS/CouchPilot"
mkdir -p "$staged_bundle/Contents/MacOS" "$staged_bundle/Contents/Resources"
go build -trimpath -ldflags='-s -w' -o "$staged_executable" ./cmd/couchpilot
cp "$root/assets/macos/Info.plist" "$staged_bundle/Contents/Info.plist"
plutil -replace LSMinimumSystemVersion -string "$deployment_target" "$staged_bundle/Contents/Info.plist"
if command -v codesign >/dev/null 2>&1; then
	identity=${CODESIGN_IDENTITY:--}
	codesign --force --deep --sign "$identity" "$staged_bundle"
fi

raw_stage=$(mktemp "$(dirname -- "$output")/.couchpilot-output.XXXXXX")
cp "$staged_executable" "$raw_stage"
chmod 0755 "$raw_stage"
mv -f -- "$raw_stage" "$output"
raw_stage=""

if [ -d "$app_bundle" ]; then
	previous_bundle="$root/bin/.CouchPilot.app.previous.$$"
	mv -- "$app_bundle" "$previous_bundle"
fi
if ! mv -- "$staged_bundle" "$app_bundle"; then
	if [ -n "$previous_bundle" ] && [ -d "$previous_bundle" ]; then
		mv -- "$previous_bundle" "$app_bundle"
		previous_bundle=""
	fi
	exit 1
fi
if [ -n "$previous_bundle" ]; then
	rm -rf -- "$previous_bundle"
	previous_bundle=""
fi
printf 'Built: %s\n' "$output"
printf 'App:   %s\n' "$app_bundle"
