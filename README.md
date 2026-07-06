# miniploy

`miniploy` is a small Go utility container for one Git repo -> one Docker image -> one Docker Compose service.

It clones a repo, checks the target branch for new commits, builds the repo Dockerfile, tags the image with a stable tag plus the commit SHA, and asks Docker Compose to recreate the managed service.

## Quick start

See:

- `compose.example.yml` for a generic template

Start only miniploy initially:

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
| `COMPOSE_FILE` | no | `/compose/docker-compose.yml` | Compose file path inside the miniploy container. Usually mount your stack directory at `/compose`. |
| `COMPOSE_PROJECT_NAME` | yes | | Explicit Compose project name. Important to avoid duplicate stacks. |
| `COMPOSE_SERVICE` | yes | | Service to recreate after a successful build. |
| `COMPOSE_PROFILE` | no | | Compose profile to enable when validating/redeploying the service. Recommended for the managed app. |
| `REDEPLOY_ARGS` | no | `--no-deps --force-recreate` | Extra args passed after `docker compose up -d`. |

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
