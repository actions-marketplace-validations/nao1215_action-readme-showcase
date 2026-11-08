[![UnitTest](https://github.com/nao1215/action-readme-showcase/actions/workflows/unit_test.yml/badge.svg)](https://github.com/nao1215/action-readme-showcase/actions/workflows/unit_test.yml)
[![E2E](https://github.com/nao1215/action-readme-showcase/actions/workflows/e2e.yml/badge.svg)](https://github.com/nao1215/action-readme-showcase/actions/workflows/e2e.yml)
[![Action](https://github.com/nao1215/action-readme-showcase/actions/workflows/action.yml/badge.svg)](https://github.com/nao1215/action-readme-showcase/actions/workflows/action.yml)
[![tested with atago](https://img.shields.io/badge/tested%20with-atago-7c3aed?logo=data:image/svg%2Bxml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCI%2BPHBhdGggZmlsbD0iI2ZmZiIgZD0iTTMuNiA0LjIgMTEuOSAxMmwtOC4zIDcuOC0xLjktMi4yTDcuOSAxMiAxLjcgNi40eiIvPjxyZWN0IGZpbGw9IiNmZmYiIHg9IjEyLjYiIHk9IjE3LjIiIHdpZHRoPSI5LjciIGhlaWdodD0iMi44IiByeD0iMS40Ii8%2BPC9zdmc%2B&logoColor=white)](https://github.com/nao1215/atago)
![GitHub](https://img.shields.io/github/license/nao1215/action-readme-showcase)

# action-readme-showcase

A GitHub Action that draws a card for each repository you list, plus an optional star history graph, and puts them in your README. The cards are SVG files in your own repository with a light and a dark version, so they follow the reader's GitHub theme and do not depend on an image service being up.

<picture><source media="(prefers-color-scheme: dark)" srcset="doc/img/dark/nao1215__gup.svg"><img src="doc/img/light/nao1215__gup.svg" width="400" alt="nao1215/gup"></picture>
<picture><source media="(prefers-color-scheme: dark)" srcset="doc/img/dark/nao1215__sqly.svg"><img src="doc/img/light/nao1215__sqly.svg" width="400" alt="nao1215/sqly"></picture>

<picture><source media="(prefers-color-scheme: dark)" srcset="doc/img/star-history-dark.svg"><img src="doc/img/star-history-light.svg" width="808" alt="Star history"></picture>

## Usage

Put two markers in your README. Everything between them is replaced on each run; the rest of the file is left alone.

```markdown
<!-- showcase:start -->
<!-- showcase:end -->
```

List the repositories in `.github/readme-showcase.yml`:

```yaml
sections:
  - groups:
      - title: Go CLI tools
        repos:
          - name: gup
            featured: true
          - name: sqly
      - title: Libraries
        repos:
          - name: markdown
          - name: golang/go
```

Then add `.github/workflows/readme-showcase.yml`:

```yaml
name: README showcase

on:
  schedule:
    - cron: "17 3 * * *"
  workflow_dispatch:

permissions:
  contents: write

jobs:
  showcase:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7

      - id: showcase
        uses: nao1215/action-readme-showcase@v0

      - if: steps.showcase.outputs.changed == 'true'
        run: |
          git config user.name "github-actions[bot]"
          git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
          git add README.md cards
          git commit -m "docs: update showcase"
          git push
```

The cards go to `cards/light/` and `cards/dark/`. Run the workflow once from the Actions tab to fill the README right away.

## Star history

Add `starHistory` to the config to draw how stars accumulated over time. It draws one line per featured repository that has at least `minStars` stars, six lines at most.

```yaml
starHistory:
  minStars: 100
sections:
  ...
```

The graph needs a personal access token. The token GitHub Actions provides can read star counts, which is all the cards need, but GitHub refuses it when listing who starred a repository and when. Without an extra token the action stops with an error that says so.

1. Open [Settings > Developer settings > Fine-grained tokens](https://github.com/settings/personal-access-tokens/new).
2. Set Repository access to "Public repositories". Add no permissions; reading public stars needs none.
3. Pick an expiration and generate the token.
4. In the repository that runs the workflow, add it under Settings > Secrets and variables > Actions as `STARS_TOKEN`, or from the shell:

   ```shell
   gh secret set STARS_TOKEN
   ```

5. Pass it to the action:

   ```yaml
   - id: showcase
     uses: nao1215/action-readme-showcase@v0
     with:
       token: ${{ secrets.STARS_TOKEN }}
   ```

The token is used only for reading from the GitHub API. The push in the last step still uses the token from `actions/checkout`. When the token expires, the run fails, and you replace the secret.

Each run makes one GraphQL request for the cards and one REST request per 100 stars of each repository in the graph.

## Config

| Key | Default | Meaning |
| --- | --- | --- |
| `owner` | owner of the repository running the workflow | Owner for names written without `owner/`. |
| `starHistory.minStars` | no graph | Featured repositories with at least this many stars get a line in the graph. |
| `sections[].summary` | none | Folds the section into `<details>` with this summary. |
| `sections[].groups[].title` | required | Heading above the cards. |
| `repos[].name` | required | `repo` or `owner/repo`. |
| `repos[].description` | GitHub description | Text on the card. Backticks are dropped. |
| `repos[].featured` | `false` | Gold border and a "featured" badge. |
| `repos[].language` | GitHub primary language | `{name: gozen, color: "#ff5c8a"}` to override the label and dot color. |

Cards come two to a row in the order you list them. A misspelled key is an error rather than being ignored. JSON works too, since it is valid YAML.

## Inputs

| Name | Default | Description |
| --- | --- | --- |
| `token` | `${{ github.token }}` | Token for the GitHub API. Needs to be a personal access token for the star history graph. |
| `config` | `.github/readme-showcase.yml` | Config file. |
| `readme` | `README.md` | README to rewrite. |
| `cards-dir` | `cards` | Where the SVG files go, relative to the README. |
| `version` | the release the action is referenced at | readme-showcase release to run, such as `v0.1.0` or `latest`. |

## Outputs

| Name | Description |
| --- | --- |
| `changed` | `true` when a card, the graph, or the README changed. |
| `version` | The readme-showcase release that ran. |

## How it runs

The action downloads the prebuilt `readme-showcase` binary for the runner (Linux, macOS, or Windows on amd64 or arm64) from this repository's release, checks it against `checksums.txt`, and runs it. `@v0` resolves to the newest `v0.x.y` release. Referencing a branch or a commit SHA builds the binary from source with the Go that GitHub's runner images include.

The binary also runs outside Actions:

```shell
go install github.com/nao1215/action-readme-showcase@latest
GITHUB_TOKEN=$(gh auth token) action-readme-showcase -config .github/readme-showcase.yml
```

On GitHub Enterprise Server, `GITHUB_API_URL` and `GITHUB_GRAPHQL_URL`, which Actions sets, point it at the server's API.

## Development

```shell
make test   # unit tests
make e2e    # atago specs against a stub GitHub API, no network needed
make lint   # golangci-lint, shellcheck, actionlint
```

To release, push a `vX.Y.Z` tag. The Release workflow builds the binaries into a draft release. Open the draft, tick "Publish this Action to the GitHub Marketplace", and publish it; that moves the `v0` and `v0.x` tags to the new release.

## License

[MIT](./LICENSE)
