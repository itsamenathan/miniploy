<div align="center">
  <img src="assets/miniploy-icon.png" alt="miniploy icon" width="160">

# miniploy

</div>

`miniploy` is a small Go utility container for one Git repo -> one Docker image -> one Docker Compose service.

It clones a repo, checks the target branch for new commits, builds the repo Dockerfile, tags the image with a stable tag plus the commit SHA, and asks Docker Compose to recreate the managed service. It also tracks the configured Compose file and redeploys the service when that file changes, even if the Git commit is unchanged.

## Quick start

See:

- `example/` for a complete self-contained nginx deployment
- `compose.example.yml` for a generic template

Run the included nginx example from the repo root:

```bash
docker compose -f example/compose.yaml up -d miniploy
```

For your own stack, start only miniploy initially:

```bash
docker compose up -d miniploy
```

You can control the managed app through `miniployctl` from inside the miniploy container:

```bash
docker compose exec miniploy miniployctl status
docker compose exec miniploy miniployctl health
docker compose exec miniploy miniployctl logs -f
docker compose exec miniploy miniployctl redeploy
docker compose exec miniploy miniployctl rebuild
```

The managed app should live behind a Compose profile so first boot does not fail before the image exists:

```yaml
app:
  profiles: [app]
  image: private-app:live
```

## Supported environment variables

### Git

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `GIT_URL` | yes | | Git repository URL to clone, e.g. `https://github.com/org/app.git` or `git@github.com:org/app.git`. |
| `GIT_BRANCH` | no | `main` | Branch to watch and deploy. |
| `GIT_AUTH_MODE` | no | `none` | Git auth mode. Supported values: `none`, `ssh`. |
| `GIT_SSH_KEY_PATH` | only for SSH | | Path to a mounted SSH private key when `GIT_AUTH_MODE=ssh`. |

### Docker build

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `IMAGE_NAME` | yes | | Stable image tag Compose should run, e.g. `my-app:live`. |
| `DOCKERFILE` | no | `Dockerfile` | Dockerfile path relative to the cloned repo root. |
| `BUILD_CONTEXT` | no | `.` | Build context path relative to the cloned repo root. |

Miniploy also tags each successful build with the short commit SHA, e.g. `my-app:abc123def456`.

BuildKit is enabled for builds so Dockerfiles using `RUN --mount=type=cache` work.

### Docker Compose redeploy

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `COMPOSE_FILE` | no | `/compose/compose.yaml` | Compose file path inside the miniploy container. Usually mount your stack directory at `/compose`. |
| `COMPOSE_PROJECT_NAME` | yes | | Explicit Compose project name. Important to avoid duplicate stacks. |
| `COMPOSE_SERVICE` | yes | | Service to recreate after a successful build. |
| `COMPOSE_PROFILE` | no | | Compose profile to enable when validating/redeploying the service. Recommended for the managed app. |
| `REDEPLOY_ARGS` | no | `--no-deps --force-recreate` | Extra args passed after `docker compose up -d`. |

Miniploy runs Docker Compose from inside the miniploy container. That means `COMPOSE_FILE` is a container path, not necessarily the path on your host. The common pattern is to mount the directory containing your Compose file at `/compose`:

```yaml
services:
  miniploy:
    volumes:
      - ./:/compose:ro
    environment:
      COMPOSE_FILE: /compose/compose.yaml
```

With that mount, a host file at `./compose.yaml` is visible inside miniploy as `/compose/compose.yaml`, which is why that is the default. If your stack still uses `docker-compose.yml`, set `COMPOSE_FILE=/compose/docker-compose.yml` explicitly.

Use full host paths for app service bind mounts, e.g. `/srv/my-app/app-data:/data`. Avoid relative bind mounts like `./app-data:/data` for services managed by miniploy.

Docker Compose expands relative paths before it talks to the Docker daemon. Since Compose is running inside miniploy, `./app-data` is resolved using miniploy's filesystem, and the resulting absolute path is sent through the Docker socket to the host daemon. With the default `/compose` mount, Docker may receive `/compose/app-data:/data`; the host daemon interprets `/compose/app-data` as a host path, not as a path inside miniploy.

Miniploy needs the Compose file because it does not manually create the app container. It lets Compose recreate the service so Compose remains responsible for ports, networks, volumes, environment variables, restart policies, and healthchecks.

Miniploy stores a hash of `COMPOSE_FILE` in its state. On each poll, if the Compose file content changed but the Git commit did not, miniploy validates the Compose config and runs `docker compose up -d ... "$COMPOSE_SERVICE"` without rebuilding the image.

`COMPOSE_PROJECT_NAME` should be explicit and should match the stack you want miniploy to manage. Without a stable project name, Compose may infer different names depending on the working directory and accidentally create duplicate containers, networks, or volumes.

`COMPOSE_SERVICE` is the service to recreate after a successful image build. `COMPOSE_PROFILE` is needed when that service is hidden behind a profile, which is recommended to avoid first-boot failures before the image exists.

After a successful build, miniploy runs roughly:

```bash
docker compose \
  -f "$COMPOSE_FILE" \
  -p "$COMPOSE_PROJECT_NAME" \
  --profile "$COMPOSE_PROFILE" \
  up -d $REDEPLOY_ARGS "$COMPOSE_SERVICE"
```

## Controlling the deployed app

The image includes `miniployctl`, a helper CLI that uses the same `COMPOSE_FILE`, `COMPOSE_PROJECT_NAME`, `COMPOSE_PROFILE`, and `COMPOSE_SERVICE` environment variables as the miniploy daemon. Run it through `docker compose exec` so you do not need to remember the compose project name or service name:

```bash
docker compose exec miniploy miniployctl status
docker compose exec miniploy miniployctl health
docker compose exec miniploy miniployctl ps
docker compose exec miniploy miniployctl logs -f
docker compose exec miniploy miniployctl restart
docker compose exec miniploy miniployctl stop
docker compose exec miniploy miniployctl start
docker compose exec miniploy miniployctl redeploy
docker compose exec miniploy miniployctl rebuild
docker compose exec miniploy miniployctl help
```

`miniployctl redeploy` recreates the managed service with `REDEPLOY_ARGS`; it does not rebuild the image. `miniployctl rebuild` fetches the watched Git branch, rebuilds the current remote commit, and recreates the managed service.

### Runtime/state

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `HEALTH_ENABLED` | no | `true` | Starts the local health/status HTTP server. |
| `HEALTH_ADDR` | no | `127.0.0.1:8080` | Address for health/status endpoints. The default is reachable from inside the container for Docker healthchecks but is not exposed outside the container unless you publish the port. |
| `CHECK_INTERVAL` | no | `5m` | Git/Compose check interval. Accepts seconds like `30` or Go durations like `5m`. Set to `0` to disable polling after the startup check. |
| `DEPLOY_DELAY` | no | `0` | Optional debounce delay after a Git change is detected. Accepts seconds like `30` or Go durations like `30s`. Miniploy rechecks the branch after the delay and deploys the latest commit. |
| `KEEP_BUILDS` | no | `3` | Number of successful commit-tagged images to retain in state/cleanup. Minimum `1`. |
| `DEPLOY_ON_START` | no | `true` | If true, miniploy ensures the Compose service is up during startup when no new commit exists. |
| `DATA_DIR` | no | `/data` | Persistent data directory for repo, state, lock, and prepared SSH key copies. |
| `REPO_DIR` | no | `$DATA_DIR/repo` | Clone destination. |
| `STATE_PATH` | no | `$DATA_DIR/state.json` | Deploy state file path. |
| `LOCK_DIR` | no | `$DATA_DIR/deploy.lock` | Lock directory used to prevent overlapping deploys. |
| `LOG_LEVEL` | no | `info` | Log level: `debug`, `info`, `warn`, or `error`. |

### Deployment notifications

Miniploy can send deployment notifications through [apprise-go](https://github.com/unraid/apprise-go), which supports Apprise-style notification URLs for services such as Discord, Slack, Telegram, ntfy, Gotify, Pushover, email, and many others.

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `NOTIFY_URLS` | no | | Comma-, newline-, or whitespace-separated Apprise notification URLs. Leave empty to disable notifications. |
| `NOTIFY_ON` | no | `failure` | Notification events to send. Supported values: `failure`, `success`, or `all`. Multiple values can be comma-, newline-, or whitespace-separated. |
| `NOTIFY_TITLE` | no | `miniploy` | Source label appended to notification bodies. Event titles are generated automatically, such as `✅ Deploy Success` or `❌ Deploy Failed`. |

Examples:

```yaml
environment:
  NOTIFY_URLS: ntfy://ntfy.sh/my-miniploy-topic
  NOTIFY_ON: failure,success
```

```yaml
environment:
  NOTIFY_URLS: |
    discord://webhook_id/webhook_token
    tgram://bot_token/chat_id
  NOTIFY_ON: all
```

Notification delivery failures are logged as warnings and do not fail or roll back a deployment.

`CHECK_INTERVAL` controls how often miniploy checks the watched Git branch's remote commit and the mounted Compose file hash. Polls use a small jitter of ±10% so multiple miniploy instances that start together do not keep checking at exactly the same time. Use `30s` when you want fast personal/dev updates, `1m`-`5m` for normal use, and `10m` or more when running many services. Set `CHECK_INTERVAL=0` for manual-only operation after startup; you can still run `miniployctl rebuild` or `miniployctl redeploy` on demand.

Miniploy logs the next scheduled Git/Compose check time at debug level before each sleep.

`DEPLOY_DELAY` is useful when a branch often receives several pushes close together. When miniploy detects a Git commit change, it waits for the delay, rechecks the watched branch, and deploys the latest branch head. Force-pushes and rollbacks are also treated as normal commit changes because miniploy compares the watched branch head hash to the last deployed hash.

### Health/status endpoints

When `HEALTH_ENABLED=true`, miniploy serves local HTTP endpoints on `HEALTH_ADDR`:

- `GET /healthz` returns cheap liveness status and is used by the image `HEALTHCHECK` through `miniployctl health`.
- `GET /readyz` checks whether miniploy can currently operate, including state writability and Docker/Compose validation.
- `GET /status` returns JSON config/state details similar to `miniployctl status`, including the last failure message when present.

The Docker image uses `/healthz` rather than `/readyz` so transient Docker or Compose issues do not cause Docker to restart miniploy in a loop. Bind `HEALTH_ADDR` to a private interface and publish the port only if you want external monitoring to read these endpoints. If `HEALTH_ENABLED=false`, `miniployctl health` exits successfully so intentionally disabling the HTTP server does not make the container unhealthy.

## Release process

Development checks are defined in `mise.toml`:

```bash
mise run check
```

To prepare a release, add notes under `## [Unreleased]` in `CHANGELOG.md`, then run:

```bash
mise run release -- v0.1.1
```

The release task moves the changelog notes into a dated version section, commits the changelog update, and creates an annotated tag. Push both the branch and tag:

```bash
git push origin main
git push origin v0.1.1
```

Tag pushes matching `v*.*.*` build and publish versioned Docker images to GHCR. The GitHub Release workflow creates release notes from the matching changelog section.

## Safety

- Failed builds do not update deploy state, so the old app keeps running.
- Cleanup only removes images labeled as managed by miniploy for the configured project/service.
- The latest `KEEP_BUILDS` commit-tagged images are retained.
- Mounting `/var/run/docker.sock` gives this container high privilege; only run trusted miniploy images/config.

## Private repos

Use a read-only SSH deploy key:

```yaml
secrets:
  git_ssh_key:
    file: ./deploy-key
```

```yaml
environment:
  GIT_AUTH_MODE: ssh
  GIT_SSH_KEY_PATH: /run/secrets/git_ssh_key
secrets:
  - git_ssh_key
```

The key is copied into `DATA_DIR/.ssh/git_key` with restrictive permissions if the mounted secret is too permissive for OpenSSH.
