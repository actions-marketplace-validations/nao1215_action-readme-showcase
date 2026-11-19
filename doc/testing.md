# Testing

`make test` runs the unit tests, `make e2e` the atago specs against a stub GitHub API, and `make coverage` both. The long fuzz and property searches run on a schedule in CI, and the `Action` workflow runs the action itself on Linux, macOS and Windows runners.

## Unit and end-to-end tests

`main_test.go`, `showcase_test.go`, `stars_test.go` and `total_test.go` drive the program against an `httptest` server that answers like the GitHub API. `e2e/atago/showcase.atago.yaml` runs the built binary the way the action does: cards written and the README rewritten between the markers, another cards directory, the star history and its total, a token without access, a misspelled config key, a README without markers, no token, and `-version`.

## Golden files

`golden_test.go` renders four cards (featured with every field, a long description that wraps and is cut, counts in the thousands with a title far too long, and a bare one) and a star history graph with a total and two lines, in both themes, and compares the SVG byte for byte with `testdata/golden/`. After an intended change to the drawing, rewrite them and review the diff:

```sh
go test -run '^TestGolden_' -update
```

## Property, metamorphic and differential tests

`property_test.go` draws inputs with [rapid](https://pkg.go.dev/pgregory.net/rapid), which shrinks a failure to a small input.

- Truncation fits the width and keeps a prefix of the text, and never shows less when given more room; the text width adds up over concatenation.
- Wrapping keeps the description's words in order, with only the last one shown cut short. This property found the bug fixed in 0.3.1, where a word wider than the card was cut and the next line went on after the ellipsis.
- The compact star count is the exact count below 1,000, within 50 below 10,000, the count's thousands above, and never goes down as the count goes up; `grouped` matches the digits of `strconv.Itoa`.
- The graph's scale covers the maximum with a 1-2-5 step, and the labels keep their gap inside the plot.
- Rewriting the README between the markers twice is rewriting it once, and nothing outside the markers changes.

```sh
go test -run '^Test(Property|Metamorphic|Differential)_' -rapid.checks=20000
```

The `Property` workflow draws 20,000 inputs per test every night. A failure writes the shrunk input under `testdata/rapid/`, which git ignores and the workflow uploads.

## Fuzzing

`fuzz_test.go` holds `FuzzRenderCard` (the SVG is well-formed XML whatever GitHub returns), `FuzzReplaceBetweenMarkers`, `FuzzParseConfig` and `FuzzWrapText`. The `Fuzz` workflow searches each of them every night.

## Mutation testing

[gremlins](https://github.com/go-gremlins/gremlins) v0.6.0 changes one comparison or arithmetic operator at a time and runs the tests; a mutant no test notices is a line the tests execute without checking. The `Mutation` workflow mutates the lines a pull request changes, and the whole package once a week.

```sh
gremlins unleash --workers 2 --timeout-coefficient 30 .
```

Measured on 2026-11-19 with a 4 GiB memory cap per process:

| Tests | Killed | Lived | Not covered | Timed out | Efficacy |
|---|---|---|---|---|---|
| Before the golden files | 250 | 121 | 15 | 1 | 67.39% |
| With the golden files (now) | 320 | 51 | 15 | 1 | 86.25% |

Most of the survivors before were coordinates in `card.go` and `stargraph.go` that no test compared; the golden files compare every one of them. A pull request should not lower the efficacy below the last row.
