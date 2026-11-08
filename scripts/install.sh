#!/usr/bin/env bash
#
# install.sh puts the readme-showcase binary on the runner and prints its path
# as the bin-path step output.
#
# Which binary:
#   - the version input, when set ("latest", "v0.1.0", or "0.1.0");
#   - otherwise the release the action itself was referenced at: uses: ...@v0.1.0
#     picks v0.1.0, and a moving tag like @v0 or @v0.1 picks the release tag
#     that points at the same commit;
#   - otherwise (a branch, a commit SHA, or uses: ./ inside this repository) the
#     binary is built from the action's own source with the runner's Go.
#
# Release archives come from .goreleaser.yml:
#   readme-showcase_0.1.0_linux_amd64.tar.gz, ..._windows_arm64.zip, checksums.txt
set -euo pipefail

readonly OWNER="nao1215"
readonly REPO="action-readme-showcase"
readonly BINARY="readme-showcase"

log() { printf '%s\n' "==> $*"; }
die() { printf '%s\n' "ERROR: $*" >&2; exit 1; }

set_output() {
  if [ -n "${GITHUB_OUTPUT:-}" ]; then
    printf '%s=%s\n' "$1" "$2" >>"$GITHUB_OUTPUT"
  fi
}

gh_curl() {
  if [ -n "${INPUT_GITHUB_TOKEN:-}" ]; then
    curl --fail --silent --show-error --location --retry 3 --retry-delay 2 \
      -H "X-GitHub-Api-Version: 2022-11-28" \
      -H "Authorization: Bearer ${INPUT_GITHUB_TOKEN}" "$@"
  else
    curl --fail --silent --show-error --location --retry 3 --retry-delay 2 \
      -H "X-GitHub-Api-Version: 2022-11-28" "$@"
  fi
}

extract_tag_name() {
  local body="$1" after
  case "$body" in
    *'"tag_name"'*)
      after="${body#*\"tag_name\"}"
      after="${after#*:}"
      after="${after#*\"}"
      printf '%s' "${after%%\"*}"
      ;;
  esac
}

detect_platform() {
  case "${RUNNER_OS:-}" in
    Linux)   OS="linux"   ; EXT="tar.gz" ; BIN_SUFFIX=""     ;;
    macOS)   OS="darwin"  ; EXT="tar.gz" ; BIN_SUFFIX=""     ;;
    Windows) OS="windows" ; EXT="zip"    ; BIN_SUFFIX=".exe" ;;
    *) die "unsupported runner OS: '${RUNNER_OS:-}' (expected Linux, macOS, or Windows)" ;;
  esac
  case "${RUNNER_ARCH:-}" in
    X64|x86_64|amd64) ARCH="amd64" ;;
    ARM64|aarch64)    ARCH="arm64" ;;
    *) die "unsupported runner architecture: '${RUNNER_ARCH:-}' (expected X64 or ARM64)" ;;
  esac
}

# list_remote_tags prints "<sha> <ref>" lines for the action's tags. Tests
# replace it.
list_remote_tags() {
  git ls-remote --tags "https://github.com/${OWNER}/${REPO}.git"
}

# release_for_moving_tag prints the vX.Y.Z tag on the commit a moving tag such
# as v0 or v0.1 points at, choosing the highest when several do.
release_for_moving_tag() {
  local moving="$1" listing sha
  listing="$(list_remote_tags)" || return 1
  # An annotated tag lists twice: the tag object, and the commit as "<tag>^{}".
  # The peeled line is the commit; a lightweight tag only has the plain line.
  sha="$(printf '%s\n' "$listing" | awk -v r="refs/tags/${moving}^{}" '$2 == r {print $1}')"
  if [ -z "$sha" ]; then
    sha="$(printf '%s\n' "$listing" | awk -v r="refs/tags/${moving}" '$2 == r {print $1}')"
  fi
  [ -n "$sha" ] || return 1
  printf '%s\n' "$listing" \
    | awk -v s="$sha" '$1 == s {print $2}' \
    | sed -E 's#^refs/tags/##; s#\^\{\}$##' \
    | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' \
    | sort -V \
    | tail -n1
}

# resolve_version sets TAG ("v0.1.0") and NUM_VERSION ("0.1.0"), or leaves both
# empty when the binary has to be built from source.
resolve_version() {
  local requested ref
  requested="$(printf '%s' "${INPUT_VERSION:-}" | tr -d '[:space:]')"
  ref="${ACTION_REF:-}"
  TAG="" NUM_VERSION=""

  if [ "$requested" = "latest" ]; then
    local body
    body="$(gh_curl "https://api.github.com/repos/${OWNER}/${REPO}/releases/latest")" \
      || die "failed to query the latest ${REPO} release"
    requested="$(extract_tag_name "$body")"
    [ -n "$requested" ] || die "could not find the latest ${REPO} release"
  fi

  if [ -z "$requested" ]; then
    if [[ "$ref" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
      requested="$ref"
    elif [[ "$ref" =~ ^v[0-9]+(\.[0-9]+)?$ ]]; then
      requested="$(release_for_moving_tag "$ref")" \
        || die "could not find the release that ${ref} points at; set the version input"
      [ -n "$requested" ] || die "no vX.Y.Z release is on the commit ${ref} points at; set the version input"
    else
      return 0
    fi
  fi

  case "$requested" in
    v*) TAG="$requested"    ; NUM_VERSION="${requested#v}" ;;
    *)  TAG="v${requested}" ; NUM_VERSION="$requested"     ;;
  esac
  [[ "$NUM_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "version must look like v1.2.3, got '${requested}'"
}

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  elif command -v certutil >/dev/null 2>&1; then
    certutil -hashfile "$1" SHA256 | sed -n '2p' | tr -d '[:space:]'
  else
    return 1
  fi
}

verify_checksum() {
  local archive="$1" checksums="$2" name="$3" expected actual
  expected="$(awk -v n="$name" '$2 == n || $2 == "*"n {print $1}' "$checksums" | head -n1)"
  [ -n "$expected" ] || die "checksum for ${name} not found in checksums.txt"
  actual="$(sha256_of "$archive")" || die "no SHA-256 tool available to verify ${name}"
  expected="$(printf '%s' "$expected" | tr '[:upper:]' '[:lower:]')"
  actual="$(printf '%s' "$actual" | tr '[:upper:]' '[:lower:]')"
  [ "$expected" = "$actual" ] || die "checksum mismatch for ${name}: expected ${expected}, got ${actual}"
  log "Checksum OK (${name})"
}

extract_archive() {
  local archive="$1" dest="$2"
  mkdir -p "$dest"
  case "$archive" in
    *.tar.gz) tar -xzf "$archive" -C "$dest" ;;
    *.zip)
      if command -v unzip >/dev/null 2>&1; then
        unzip -oq "$archive" -d "$dest"
      else
        local ps
        ps="$(command -v pwsh || command -v powershell)" || die "cannot extract ${archive}: neither unzip nor PowerShell is available"
        "$ps" -NoProfile -NonInteractive -Command "Expand-Archive -Path '${archive}' -DestinationPath '${dest}' -Force"
      fi
      ;;
    *) die "unsupported archive format: ${archive}" ;;
  esac
}

download() {
  local dest_dir="$1"
  local archive_name="${BINARY}_${NUM_VERSION}_${OS}_${ARCH}.${EXT}"
  local base_url="https://github.com/${OWNER}/${REPO}/releases/download/${TAG}"
  local work="${dest_dir}/download"
  mkdir -p "$work"

  log "Downloading ${base_url}/${archive_name}"
  gh_curl -o "${work}/checksums.txt" "${base_url}/checksums.txt" \
    || die "could not download checksums.txt for ${TAG}"
  gh_curl -o "${work}/${archive_name}" "${base_url}/${archive_name}" \
    || die "could not download ${archive_name}; does release ${TAG} exist?"
  verify_checksum "${work}/${archive_name}" "${work}/checksums.txt" "$archive_name"

  extract_archive "${work}/${archive_name}" "${work}/extracted"
  local src
  src="$(find "${work}/extracted" -type f -name "${BINARY}${BIN_SUFFIX}" | head -n1)"
  [ -n "$src" ] || die "${BINARY}${BIN_SUFFIX} is not in ${archive_name}"
  cp "$src" "${dest_dir}/${BINARY}${BIN_SUFFIX}"
  chmod +x "${dest_dir}/${BINARY}${BIN_SUFFIX}" 2>/dev/null || true
  rm -rf "$work"
}

# find_go prints a go command: the one on PATH, or else the newest in the
# runner's tool cache. The macOS images keep Go only in the tool cache.
find_go() {
  if command -v go >/dev/null 2>&1; then
    command -v go
    return 0
  fi
  local cache="${RUNNER_TOOL_CACHE:-}" found
  [ -n "$cache" ] && [ -d "${cache}/go" ] || return 1
  found="$(find "${cache}/go" -path "*/bin/go${BIN_SUFFIX}" -type f 2>/dev/null | sort -V | tail -n1)"
  [ -n "$found" ] || return 1
  printf '%s\n' "$found"
}

build_from_source() {
  local dest_dir="$1" src="${GITHUB_ACTION_PATH:-}" go
  [ -n "$src" ] || die "GITHUB_ACTION_PATH is not set"
  go="$(find_go)" \
    || die "the action was not referenced at a release tag, so it builds from source, but Go was not found; reference a release (uses: ${OWNER}/${REPO}@v0), set the version input, or run actions/setup-go first"
  log "Building ${BINARY} from ${src} ($("$go" version))"
  (cd "$src" && CGO_ENABLED=0 "$go" build -trimpath -o "${dest_dir}/${BINARY}${BIN_SUFFIX}" .)
}

main() {
  detect_platform
  resolve_version

  local dest_dir="${RUNNER_TEMP:-${TMPDIR:-/tmp}}/readme-showcase"
  mkdir -p "$dest_dir"
  if [ -n "$TAG" ]; then
    download "$dest_dir"
    set_output "version" "$TAG"
  else
    build_from_source "$dest_dir"
    set_output "version" "source"
  fi
  set_output "bin-path" "${dest_dir}/${BINARY}${BIN_SUFFIX}"
  log "Installed ${dest_dir}/${BINARY}${BIN_SUFFIX}"
}

if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
  main "$@"
fi
