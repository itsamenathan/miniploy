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
4. Runs `docker compose up -d --wait` for the service you selected. If the service has a health check, miniploy waits for it to become healthy.
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

Copy [`compose.example.yml`](compose.example.yml) to `compose.yaml` in your deployment directory. It uses the published miniploy image and is configured for a public Git repository. Replace the placeholder `your-org/your-app` URL, image name, project name, and app port with your own values. To use a private repository, follow [Private repositories](#private-repositories).

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

Use the same `my-app:live` value for miniploy's `IMAGE_NAME` setting. Miniploy checks this at startup. Configure your app's networks, volumes, environment variables, health check, and ports here as usual. A health check lets miniploy tell whether the app is ready to serve traffic.

### 2. Configure miniploy

Set these required values in the `miniploy` service:

| Setting | Example | Purpose |
| --- | --- | --- |
| `GIT_URL` | `https://github.com/acme/my-app.git` | Repository to build. |
| `IMAGE_NAME` | `my-app:live` | Stable image tag used by the app service. |
| `COMPOSE_PROJECT_NAME` | `my-app` | A fixed Compose project name for this stack. |
| `COMPOSE_SERVICE` | `app` | The service miniploy recreates after a build. |

The template uses the published image:

```yaml
image: ghcr.io/itsamenathan/miniploy:latest
```

You can pin this to a release tag for repeatable upgrades. To build miniploy from a local clone, replace `image:` with `build: .` and keep the Compose file at the repository root.

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

Start **only** miniploy:

```bash
docker compose up -d miniploy
docker compose logs -f miniploy
```

Miniploy builds the app image, then starts the profiled application service. Starting `app` yourself before the first build will fail because its image does not exist yet.

Check the result with `docker compose exec miniploy miniployctl status`. The command shows the last deployment and the latest check separately. If a build is slow, `docker compose logs -f miniploy` shows its progress.

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

# Pause automatic checks, then stop the app until you resume them
docker compose exec miniploy miniployctl pause
docker compose exec miniploy miniployctl stop
```

When you are ready to deploy again, run `docker compose exec miniploy miniployctl resume`. The next automatic check runs at the configured interval. `stop` alone is temporary because the next automatic check restarts a stopped service. Pausing survives miniploy container restarts. Manual `rebuild` still works while paused. Other available commands are `ps`, `restart`, and `start`:

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
| `DEPLOY_WAIT_TIMEOUT` | No | `60s` | Maximum time to wait for the app to run or become healthy. Use seconds (`90`) or a Go duration (`90s`). |

Miniploy runs Docker Compose inside its own container. Avoid relative host bind mounts in the managed service, such as `./data:/data`: Docker resolves them from miniploy's `/compose` directory, then the host Docker daemon interprets that path on the host. Prefer full host paths, such as `/srv/my-app/data:/data`.

After each `docker compose up -d --wait`, miniploy verifies that the managed service has a running container. With a Docker health check, Compose also waits for the service to become healthy. Miniploy records a failure if it exits or stays unhealthy beyond `DEPLOY_WAIT_TIMEOUT`. On later checks, it recreates the service when it is absent or stopped, even when Git and Compose configuration are unchanged. If the stable `IMAGE_NAME` tag was removed, miniploy rebuilds it before recreating the service.

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

`/status` reports the latest check separately from the last deployment, so a Git or Compose check failure cannot leave the overall status showing success. It also reports whether automatic checks are paused. Git URL credentials are redacted. The default address is private to the container. If you publish it for external monitoring, restrict access through your network or reverse proxy because the endpoint does not require authentication and still shows deployment details.

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
      GIT_URL: git@github.com:your-org/private-app.git
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
| The app starts but deployment fails | Check its Docker health check and `DEPLOY_WAIT_TIMEOUT`, then inspect `miniployctl logs -f`. |
| `IMAGE_NAME must match` appears | Use the exact same image tag in the app service's `image:` field and miniploy's `IMAGE_NAME`. |
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
