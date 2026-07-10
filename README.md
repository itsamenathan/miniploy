<div align="center">
  <img src="assets/miniploy-icon.png" alt="miniploy icon" width="160">

# miniploy

**Build and redeploy one Docker Compose service whenever a Git branch changes.**
</div>

Miniploy is a small container that watches one Git repository, builds its Dockerfile, and asks Docker Compose to recreate one service with the new image. It is intended for simple self-hosted deployments where you want automatic updates without a full CI/CD platform.

## What miniploy does

1. Checks the configured branch for a new commit.
2. Clones or updates the repository in its persistent data volume.
3. Builds the repository's Dockerfile and tags the image with both a stable tag (for example, `my-app:live`) and the commit SHA.
4. Runs `docker compose up -d` for the service you selected.
5. Repeats at the configured interval.

It also redeploys when the **effective Docker Compose configuration** changes, even if the Git commit does not.

> Miniploy manages one repository, one image, and one Compose service per container. Run another miniploy container for another application.

## Before you start

You need:

- A Linux host with Docker Engine and the Docker Compose plugin.
- A repository containing a Dockerfile that builds your application.
- A Compose file that defines both miniploy and the application service.
- Permission to mount `/var/run/docker.sock` into miniploy.

Docker socket access is effectively host-level access. Use only a trusted miniploy image, Compose file, and Git repository.

## Quick start

The included nginx example is the fastest way to see a complete deployment:

```bash
docker compose -f example/compose.yaml up -d miniploy
docker compose -f example/compose.yaml logs -f miniploy
```

After the first deployment completes, open <http://localhost:8080>. See [`example/README.md`](example/README.md) for the full walkthrough and cleanup command.

## Deploy your own application

Start with [`compose.example.yml`](compose.example.yml). If you plan to build miniploy locally, clone this repository, rename that file to `compose.yaml` in the repository root, then edit the placeholders. If you keep your deployment Compose file elsewhere, use the published miniploy image described below instead of `build: .`.

### 1. Define the application service

Your application needs a stable image name and should be placed behind a Compose profile. The profile lets miniploy start before the first image has been built.

```yaml
services:
  app:
    profiles: [app]
    image: my-app:live
    restart: unless-stopped
    ports:
      - "8080:8080"
```

Use the same `my-app:live` value for miniploy's `IMAGE_NAME` setting. Configure your app's networks, volumes, environment variables, health check, and ports here as usual.

### 2. Configure miniploy

Set these required values in the `miniploy` service:

| Setting | Example | Purpose |
| --- | --- | --- |
| `GIT_URL` | `git@github.com:acme/my-app.git` | Repository to build. |
| `IMAGE_NAME` | `my-app:live` | Stable image tag used by the app service. |
| `COMPOSE_PROJECT_NAME` | `my-app` | A fixed Compose project name for this stack. |
| `COMPOSE_SERVICE` | `app` | The service miniploy recreates after a build. |

The generic template builds miniploy from the current directory. To use the published image instead, replace `build: .` with:

```yaml
image: ghcr.io/itsamenathan/miniploy:latest
```

Keep these mounts:

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock
  - ./:/compose:ro
  - miniploy-data:/data
```

- The Docker socket lets miniploy build images and run Docker Compose on the host.
- The read-only Compose mount lets miniploy validate and apply your stack configuration.
- The named data volume preserves the cloned repository and deployment state across restarts.

### 3. Start miniploy

Build it first if you kept `build: .` in the template:

```bash
docker compose build miniploy
```

Then start **only** miniploy:

```bash
docker compose up -d miniploy
docker compose logs -f miniploy
```

Miniploy builds the app image, then starts the profiled application service. Starting `app` yourself before the first build will fail because its image does not exist yet.

## Daily operations

Run these from the directory containing your Compose file:

```bash
# Current deployment state and application container status
docker compose exec miniploy miniployctl status

# Follow application logs
docker compose exec miniploy miniployctl logs -f

# Check miniploy's liveness endpoint
docker compose exec miniploy miniployctl health

# Recreate the app using the current image; does not build
docker compose exec miniploy miniployctl redeploy

# Fetch the watched branch, rebuild, and recreate the app
docker compose exec miniploy miniployctl rebuild
```

Other available commands are `ps`, `restart`, `stop`, and `start`:

```bash
docker compose exec miniploy miniployctl help
```

## Configuration reference

All configuration is supplied through environment variables. Values that are supplied with an invalid boolean, integer, or duration format cause startup to fail rather than silently falling back to a default.

### Git source

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `GIT_URL` | Yes | — | Repository URL, such as `https://github.com/acme/app.git` or `git@github.com:acme/app.git`. |
| `GIT_BRANCH` | No | `main` | Branch to watch and deploy. |
| `GIT_AUTH_MODE` | No | `none` | Authentication mode: `none` or `ssh`. |
| `GIT_SSH_KEY_PATH` | With SSH | — | Path to the mounted private key when `GIT_AUTH_MODE=ssh`. |

### Build

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `IMAGE_NAME` | Yes | — | Stable image tag your app service uses. |
| `DOCKERFILE` | No | `Dockerfile` | Dockerfile path relative to the cloned repository. |
| `BUILD_CONTEXT` | No | `.` | Docker build context relative to the cloned repository. |

Successful builds are also tagged with the first 12 characters of their Git commit. BuildKit is enabled, so Dockerfiles using `RUN --mount=type=cache` work.

### Compose deployment

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `COMPOSE_FILE` | No | `/compose/compose.yaml` | Path to the Compose file *inside the miniploy container*. |
| `COMPOSE_PROJECT_NAME` | Yes | — | Stable Compose project name for the stack. |
| `COMPOSE_SERVICE` | Yes | — | Service to recreate after a successful build. |
| `COMPOSE_PROFILE` | No | — | Profile to enable when validating and starting the managed service. |
| `REDEPLOY_ARGS` | No | `--no-deps --force-recreate` | Extra arguments passed to `docker compose up -d`. |

Miniploy runs Docker Compose inside its own container. Avoid relative host bind mounts in the managed service, such as `./data:/data`: Docker resolves them from miniploy's `/compose` directory, then the host Docker daemon interprets that path on the host. Prefer full host paths, such as `/srv/my-app/data:/data`.

After each `docker compose up -d`, miniploy verifies that the managed service has a running container. It records a deployment failure instead of reporting success if the container exits immediately. On later checks, it also recreates the service when it is absent or stopped, even when Git and Compose configuration are unchanged. If the stable `IMAGE_NAME` tag was removed, miniploy rebuilds it before recreating the service.

### Runtime and retention

| Variable | Default | Description |
| --- | --- | --- |
| `CHECK_INTERVAL` | `5m` | How often to check Git and Compose configuration. Use seconds (`30`) or Go durations (`5m`). Set `0` for manual-only operation after the startup check. |
| `DEPLOY_DELAY` | `0` | Optional delay after detecting a change. Miniploy checks again after the delay and deploys the latest branch head. |
| `KEEP_BUILDS` | `3` | Number of successful commit-tagged images to retain. Minimum `1`. |
| `DEPLOY_ON_START` | `true` | Ensure the service is running during startup even if no new commit exists. |
| `DATA_DIR` | `/data` | Persistent directory for the repository, state, lock, and prepared SSH key. |
| `REPO_DIR` | `$DATA_DIR/repo` | Clone destination. |
| `STATE_PATH` | `$DATA_DIR/state.json` | Deployment-state file. |
| `LOCK_DIR` | `$DATA_DIR/deploy.lock` | Deployment lock path. |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error`. |

### Health and status endpoints

| Variable | Default | Description |
| --- | --- | --- |
| `HEALTH_ENABLED` | `true` | Enables the local HTTP health server. |
| `HEALTH_ADDR` | `127.0.0.1:8080` | Bind address for health endpoints. |

When enabled, miniploy exposes:

- `GET /healthz` — liveness check; this is what the image health check uses.
- `GET /readyz` — readiness check; also verifies writable state plus Docker and Compose access.
- `GET /status` — JSON configuration and last deployment status.

The default address is private to the container. Bind and publish a different address only if external monitoring needs it.

### Notifications

Miniploy sends optional deployment notifications using [apprise-go](https://github.com/unraid/apprise-go). This supports Apprise URLs for services such as Discord, Slack, Telegram, ntfy, Gotify, Pushover, and email.

| Variable | Default | Description |
| --- | --- | --- |
| `NOTIFY_URLS` | — | Comma-, whitespace-, or newline-separated Apprise URLs. Empty disables notifications. |
| `NOTIFY_ON` | `failure` | Events to send: `failure`, `success`, or `all`. |
| `NOTIFY_TITLE` | `miniploy` | Label included in notification bodies. |

For example:

```yaml
environment:
  NOTIFY_URLS: ntfy://ntfy.sh/my-miniploy-topic
  NOTIFY_ON: success,failure
```

Notification delivery failures are logged but do not roll back a deployment.

## Private repositories

Use a read-only SSH deploy key rather than placing a token in the Git URL.

```yaml
secrets:
  git_ssh_key:
    file: ./deploy-key

services:
  miniploy:
    environment:
      GIT_AUTH_MODE: ssh
      GIT_SSH_KEY_PATH: /run/secrets/git_ssh_key
    secrets:
      - git_ssh_key
```

If the mounted key's permissions are too broad for OpenSSH, miniploy copies it into its persistent data directory with restrictive permissions.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| Miniploy exits immediately | Run `docker compose logs miniploy`. Confirm all required variables are set and duration/boolean/integer values are valid. |
| The app does not start on first boot | Confirm the app service has the configured `COMPOSE_PROFILE`, and start only `miniploy` initially. |
| A Git push does not deploy | Confirm `GIT_BRANCH`, wait for `CHECK_INTERVAL`, then inspect `docker compose logs -f miniploy`. Use `miniployctl rebuild` to force a build. |
| Compose changes are ignored | Ensure miniploy can read the mounted Compose directory and that `COMPOSE_FILE` points to the correct container path. |
| Git cannot access a private repository | Check deploy-key permissions, `GIT_AUTH_MODE=ssh`, and `GIT_SSH_KEY_PATH`. |
| Docker or Compose validation fails | Confirm the Docker socket mount is present and the container can access the Compose file. |

## Development and releases

For contributors, the local quality suite is:

```bash
mise run check
```

To prepare a release, add notes under `## [Unreleased]` in `CHANGELOG.md`, then run:

```bash
mise run release -- v0.1.1
git push origin main
git push origin v0.1.1
```

Version tags publish Docker images to GHCR and create a GitHub Release.
