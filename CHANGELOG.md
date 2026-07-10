# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html) when versioned releases are tagged.

## [Unreleased]

## [0.3.1] - 2026-07-10

### Fixed

- Verify that a Compose service has a running container after `docker compose up -d` rather than reporting a successful redeploy when it exits immediately.
- Reconcile stopped or removed managed services during unchanged polling cycles, and rebuild the stable image tag when it was removed before recreating the service.

## [0.3.0] - 2026-07-10

### Added

- Add advisory deployment locking that is automatically released when miniploy exits, including compatibility with legacy lock directories.
- Detect effective Docker Compose configuration changes by hashing rendered Compose configuration.
- Add focused tests for Compose configuration hashing, deployment-state persistence failures, locking, command credential redaction, and invalid configuration.

### Changed

- Rewrite the README as an end-to-end operator guide with setup, configuration, operations, troubleshooting, health, notifications, and private-repository guidance.
- Make the generic Compose template mount the Compose directory read-only.
- Reject malformed supplied boolean, duration, and integer environment values during startup instead of silently using defaults.

### Fixed

- Surface state persistence failures while recording deployment attempts and deployment failures.
- Redact HTTP(S) URL credentials from command errors and stop logging full command output to avoid exposing resolved Compose secrets.

## [0.2.0] - 2026-07-09

### Added

- Add deployment notifications through apprise-go with `NOTIFY_URLS`, `NOTIFY_ON`, and `NOTIFY_TITLE` configuration.
- Add success, redeploy success, and failure notification events with emoji-formatted titles and message bodies.
- Add project icon to the README.

### Changed

- Bump the Go toolchain and Docker build image to Go 1.25 for apprise-go support.

## [0.1.0] - 2026-07-08

### Added

- Add CI workflow that runs the mise `check` task on pushes and pull requests.
- Add GitHub Release workflow that publishes tag releases using matching `CHANGELOG.md` release notes.
- Add release preparation script and `mise run release -- vX.Y.Z` task for changelog updates, release commits, and annotated tags.
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
