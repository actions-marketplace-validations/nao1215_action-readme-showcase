# Changelog

All notable changes to this project are documented in this file. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this
project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
