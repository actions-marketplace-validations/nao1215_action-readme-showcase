# Changelog

All notable changes to this project are documented in this file. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this
project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- A card's description no longer goes on after an ellipsis. A word wider than the card (a long URL or path) was cut with an ellipsis on the first line and the rest of the description followed on the second; the cut line now ends the description, as the ellipsis says.

### Changed

- CI scans the repository with gosec and Trivy on every pull request (Trivy covers dependency vulnerabilities, committed secrets and misconfigurations, and also runs daily). Workflow actions are recorded in `.github/workflows/actions.lock`, which CI verifies against the pinned commits.
- CI scans the commits of pull requests and of pushes to main for committed secrets with gitleaks (Dependabot pull requests are skipped: their read-only token cannot list the commits), and checks that every module linked into the released binary has an allowed license with go-licenses. The end-to-end tests run on atago v0.26.3 (setup-atago v0.1.5), govulncheck is v1.10.0, releases are built with GoReleaser v2.20.2 and signed with cosign-installer v4.1.3, and CodeQL is v4.39.3. A new push to a pull request cancels the run it supersedes.

## [0.3.0] - 2026-11-09

### Added

- `starHistory.total: true` draws the combined history of all public repositories owned by `owner`, independently of individual graph lines and cards. Forks and archived repositories are included; private and other-owner repositories are excluded.
- Graphs and README image descriptions identify the total's scope. Public repository listing is paginated, shared histories are fetched once, and failed reads never produce a partial total.

### Changed

- Removed `starHistory.minStars`. Delete this key from existing configurations; every repository in `starHistory.repos` is now selected regardless of its star count.
- With total enabled, the graph shows the total plus up to five individual lines. The display limit does not restrict the total.
- Total graphs use a distinct README image URL so switching from individual-only graphs refreshes cached images.

## [0.2.0] - 2026-11-08

### Fixed

- Read aggregate star history with Metadata read permission instead of the restricted stargazers list. Graphs use daily counts and no longer require access to individual stargazers.

### Changed

- Star history now requires `starHistory.repos`, independent of card highlighting. Add an explicit list when upgrading from v0.1.0; graph-only repositories are supported.

## [0.1.0] - 2026-11-08

### Added

- First release. Draws repository cards and a star history graph as light and
  dark SVG files and rewrites the README between `<!-- showcase:start -->` and
  `<!-- showcase:end -->`. Based on the generator behind the
  [nao1215 profile README](https://github.com/nao1215/nao1215).
