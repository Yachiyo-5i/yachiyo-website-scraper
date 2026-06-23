#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(git -C "$SCRIPT_DIR" rev-parse --show-toplevel 2>/dev/null || echo "$SCRIPT_DIR")"

DEFAULT_NAME="yachiyo-playwright-fetcher"
DEFAULT_TAG="latest"
DEFAULT_PLATFORMS="linux/amd64,linux/arm64"

usage() {
	cat <<'EOF'
Usage:
  ./build-docker.sh [options]

Build and push the Playwright fetcher image to GHCR.

Options:
  --owner OWNER        GHCR owner/namespace. Defaults to GHCR_OWNER or git remote owner.
  --name NAME          Image name. Defaults to yachiyo-playwright-fetcher.
  --image IMAGE        Full image repository, for example ghcr.io/yachiyo-5i/yachiyo-playwright-fetcher.
  --tag TAG            Image tag. Defaults to latest.
  --platforms LIST     Build platforms. Defaults to linux/amd64,linux/arm64.
  --push               Push the image. This is the default.
  --load               Load the image into local Docker. Only supports one platform.
  --local              Fast local docker build for the current platform.
  --no-cache-registry  Disable GHCR registry cache for buildx pushes.
  --dry-run            Print the docker buildx command without running it.
  -h, --help           Show this help.

Environment:
  GHCR_OWNER           Default GHCR owner/namespace.
  IMAGE_NAME           Default image name.
  IMAGE_TAG            Default image tag.
  PLATFORMS            Default platform list.
EOF
}

lower() {
	printf '%s' "$1" | tr '[:upper:]' '[:lower:]'
}

github_owner_from_remote() {
	local remote
	remote="$(git -C "$REPO_ROOT" remote get-url origin 2>/dev/null || true)"
	case "$remote" in
		git@*:*.git)
			remote="${remote#*:}"
			remote="${remote%.git}"
			printf '%s' "${remote%%/*}"
			;;
		https://github.com/*/*.git|http://github.com/*/*.git)
			remote="${remote#*github.com/}"
			remote="${remote%.git}"
			printf '%s' "${remote%%/*}"
			;;
		https://github.com/*/*|http://github.com/*/*)
			remote="${remote#*github.com/}"
			printf '%s' "${remote%%/*}"
			;;
	esac
}

source_url_from_remote() {
	local remote owner repo
	remote="$(git -C "$REPO_ROOT" remote get-url origin 2>/dev/null || true)"
	case "$remote" in
		git@*:*.git)
			remote="${remote#*:}"
			remote="${remote%.git}"
			owner="${remote%%/*}"
			repo="${remote#*/}"
			printf 'https://github.com/%s/%s' "$owner" "$repo"
			;;
		https://github.com/*/*.git|http://github.com/*/*.git)
			remote="${remote%.git}"
			printf '%s' "$remote"
			;;
		https://github.com/*/*|http://github.com/*/*)
			printf '%s' "$remote"
			;;
	esac
}

author="$(git -C "$REPO_ROOT" config user.name 2>/dev/null || true)"
if [[ -z "${author// }" ]]; then
	echo "error: git user.name is required for the image author label" >&2
	exit 1
fi

owner="${GHCR_OWNER:-$(github_owner_from_remote)}"
name="${IMAGE_NAME:-$DEFAULT_NAME}"
tag="${IMAGE_TAG:-$DEFAULT_TAG}"
platforms="${PLATFORMS:-$DEFAULT_PLATFORMS}"
image=""
output="--push"
dry_run=0
local_build=0
cache_registry=1

while [[ $# -gt 0 ]]; do
	case "$1" in
		--owner)
			owner="${2:-}"
			shift 2
			;;
		--name)
			name="${2:-}"
			shift 2
			;;
		--image)
			image="${2:-}"
			shift 2
			;;
		--tag)
			tag="${2:-}"
			shift 2
			;;
		--platforms)
			platforms="${2:-}"
			shift 2
			;;
		--push)
			output="--push"
			shift
			;;
		--load)
			output="--load"
			shift
			;;
		--local)
			local_build=1
			output=""
			shift
			;;
		--no-cache-registry)
			cache_registry=0
			shift
			;;
		--dry-run)
			dry_run=1
			shift
			;;
		-h|--help)
			usage
			exit 0
			;;
		*)
			echo "error: unknown option: $1" >&2
			usage >&2
			exit 1
			;;
	esac
done

if [[ -z "$image" ]]; then
	if [[ -z "${owner// }" ]]; then
		echo "error: GHCR owner could not be inferred; pass --owner or set GHCR_OWNER" >&2
		exit 1
	fi
	image="ghcr.io/$(lower "$owner")/$(lower "$name")"
else
	image="$(lower "$image")"
fi

if [[ "$output" == "--load" && "$platforms" == *,* ]]; then
	echo "error: --load only supports one platform; pass --platforms linux/amd64 or linux/arm64" >&2
	exit 1
fi

source_url="$(source_url_from_remote)"

printf 'Image: %s:%s\n' "$image" "$tag"
printf 'Author: %s\n' "$author"

if [[ "$local_build" -eq 1 ]]; then
	cmd=(
		docker build
		--file "$SCRIPT_DIR/Dockerfile"
		--tag "$image:$tag"
		--label "org.opencontainers.image.authors=$author"
		--label "org.opencontainers.image.title=$DEFAULT_NAME"
		--label "org.opencontainers.image.description=HTTP Playwright fetch helper for yachiyo scraper"
	)
	if [[ -n "$source_url" ]]; then
		cmd+=(--label "org.opencontainers.image.source=$source_url")
	fi
	cmd+=("$SCRIPT_DIR")
	printf 'Mode: local docker build\n'
else
	cmd=(
		docker buildx build
		--file "$SCRIPT_DIR/Dockerfile"
		--platform "$platforms"
		--tag "$image:$tag"
		--label "org.opencontainers.image.authors=$author"
		--label "org.opencontainers.image.title=$DEFAULT_NAME"
		--label "org.opencontainers.image.description=HTTP Playwright fetch helper for yachiyo scraper"
	)
	if [[ -n "$source_url" ]]; then
		cmd+=(--label "org.opencontainers.image.source=$source_url")
	fi
	if [[ "$output" == "--push" && "$cache_registry" -eq 1 ]]; then
		cmd+=(
			--cache-from "type=registry,ref=$image:buildcache"
			--cache-to "type=registry,ref=$image:buildcache,mode=max"
		)
	fi
	cmd+=("$output" "$SCRIPT_DIR")
	printf 'Platforms: %s\n' "$platforms"
	printf 'Output: %s\n' "$output"
	if [[ "$output" == "--push" && "$cache_registry" -eq 1 ]]; then
		printf 'Cache: %s\n' "$image:buildcache"
	fi
fi

if [[ "$dry_run" -eq 1 ]]; then
	printf 'Command:'
	printf ' %q' "${cmd[@]}"
	printf '\n'
	exit 0
fi

"${cmd[@]}"
