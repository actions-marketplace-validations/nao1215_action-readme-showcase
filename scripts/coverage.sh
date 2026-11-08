#!/usr/bin/env bash
#
# coverage.sh merges unit-test coverage with the coverage of the real binary
# run by the atago specs into one cover.out. The unit tests call run()
# directly; the specs run the built binary the way the action does, so the
# merged number covers both.
set -euo pipefail

cd "$(dirname "$0")/.."
root="$(pwd)"
cov="${root}/.coverage"
rm -rf "$cov"
mkdir -p "${cov}/unit" "${cov}/e2e" "${cov}/merged"

go test -count=1 -cover -covermode=atomic -coverpkg=./... ./... -args -test.gocoverdir="${cov}/unit"
GOCOVERDIR="${cov}/e2e" atago run --profile cover e2e/atago

go tool covdata merge -i="${cov}/unit,${cov}/e2e" -o="${cov}/merged"
go tool covdata textfmt -i="${cov}/merged" -o="${root}/cover.out"
go tool cover -func=cover.out | tail -n 1
go tool cover -html=cover.out -o cover.html
