# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html) when versioned releases are tagged.

## [Unreleased]

### Added

- Add local health/status HTTP server, enabled by default on `127.0.0.1:8080`.
- Add `GET /healthz` for cheap liveness checks.
- Add `GET /readyz` for state writability plus Docker/Compose readiness checks.
- Add `GET /status` for JSON config/state visibility.
- Add `HEALTH_ENABLED` and `HEALTH_ADDR` runtime configuration.
- Add Docker image `HEALTHCHECK` using `miniployctl health`.
- Add `miniployctl health` command.
- Add optional `DEPLOY_DELAY` debounce for Git commit changes.
- Add polling jitter of ±10% to avoid synchronized checks across many instances.
- Add `CHECK_INTERVAL=0` support for manual-only operation after startup.
- Add last failure details to state via `lastError` and `lastErrorAt`.
- Add extra `miniployctl status` output for check interval, deploy delay, Compose hash, and last error.
- Add `mise.toml` with pinned Go and golangci-lint versions plus `fmt`, `test`, `lint`, and `check` tasks.
- Add test coverage for config parsing, health endpoints, health command behavior, jitter, and state persistence/error tracking.

### Changed

- Change default `CHECK_INTERVAL` from `30s` to `5m`.
- Change example Compose files to use `CHECK_INTERVAL: 5m`.
- Move next scheduled Git/Compose check logging to debug level.
- Document polling recommendations, jitter behavior, disabled polling, deploy debounce, health endpoints, and force-push/rollback behavior.

### Fixed

- Clear stored last failure details after successful deploys and redeploys.
- Normalize `HEALTH_ADDR` values with trailing slashes when `miniployctl health` builds the `/healthz` URL.
