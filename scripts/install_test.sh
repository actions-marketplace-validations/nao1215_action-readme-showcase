#!/usr/bin/env bash
#
# install_test.sh checks how install.sh picks the binary to install, with the
# tag listing stubbed out so it runs offline.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source-path=SCRIPTDIR
# shellcheck source=install.sh
source "${here}/install.sh"

failures=0

# v0 and v0.2 are annotated moving tags on the v0.2.1 commit; v0.1.0 is older,
# and v0.2.0-rc.1 sits on the same commit but is not a release.
list_remote_tags() {
  cat <<'LIST'
1111111111111111111111111111111111111111	refs/tags/v0
aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa	refs/tags/v0^{}
2222222222222222222222222222222222222222	refs/tags/v0.2
aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa	refs/tags/v0.2^{}
bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb	refs/tags/v0.1.0
aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa	refs/tags/v0.2.0-rc.1
aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa	refs/tags/v0.2.1
cccccccccccccccccccccccccccccccccccccccc	refs/tags/v1
LIST
}

check() {
  local version="$1" ref="$2" want="$3"
  INPUT_VERSION="$version" ACTION_REF="$ref"
  resolve_version
  if [ "$TAG" != "$want" ]; then
    printf 'FAIL: version=%-7s ref=%-9s -> %s (want %s)\n' "'$version'" "'$ref'" "'$TAG'" "'$want'"
    failures=$((failures + 1))
  else
    printf 'ok:   version=%-7s ref=%-9s -> %s\n' "'$version'" "'$ref'" "'$TAG'"
  fi
}

check_fails() {
  local version="$1" ref="$2"
  if (INPUT_VERSION="$version" ACTION_REF="$ref" resolve_version) 2>/dev/null; then
    printf 'FAIL: version=%s ref=%s should have failed\n' "'$version'" "'$ref'"
    failures=$((failures + 1))
  else
    printf 'ok:   version=%-7s ref=%-9s -> error\n' "'$version'" "'$ref'"
  fi
}

check ""       "v0.2.1" "v0.2.1"
check ""       "v0"     "v0.2.1"
check ""       "v0.2"   "v0.2.1"
check "0.1.0"  "v0"     "v0.1.0"
check "v0.1.0" ""       "v0.1.0"
# A branch, a SHA, or uses: ./ builds from source.
check ""       "main"   ""
check ""       ""       ""
check ""       "0123456789abcdef0123456789abcdef01234567" ""
# v1 points at a commit with no release tag; v9 does not exist.
check_fails "" "v1"
check_fails "" "v9"
check_fails "nightly" ""

if [ "$failures" -gt 0 ]; then
  printf '%d failure(s)\n' "$failures"
  exit 1
fi
