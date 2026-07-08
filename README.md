# miniploy

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

### Runtime/state

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `CHECK_INTERVAL` | no | `30` | Poll interval. Accepts seconds like `30` or Go durations like `5m`. |
| `KEEP_BUILDS` | no | `3` | Number of successful commit-tagged images to retain in state/cleanup. Minimum `1`. |
| `DEPLOY_ON_START` | no | `true` | If true, miniploy ensures the Compose service is up during startup when no new commit exists. |
| `DATA_DIR` | no | `/data` | Persistent data directory for repo, state, lock, and prepared SSH key copies. |
| `REPO_DIR` | no | `$DATA_DIR/repo` | Clone destination. |
| `STATE_PATH` | no | `$DATA_DIR/state.json` | Deploy state file path. |
| `LOCK_DIR` | no | `$DATA_DIR/deploy.lock` | Lock directory used to prevent overlapping deploys. |
| `LOG_LEVEL` | no | `info` | Log level: `debug`, `info`, `warn`, or `error`. |

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
