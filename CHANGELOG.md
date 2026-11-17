# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
### Fixed
- Bound unnamed default-client metrics to three shared counters while preserving
  unique client names and User-Agent values and existing named-client metrics.
- Run the full local lint, race test and coverage target in CI with a pinned linter.

### Changed
- Require patched Go 1.26.11 and update compatible dependencies.

## [1.0.0] - 2023-02-08
### Added
- First release.
