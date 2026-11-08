# Changelog

All notable changes to this project are documented in this file. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this
project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
