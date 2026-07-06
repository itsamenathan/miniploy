# miniploy

`miniploy` is a small Go utility container for one Git repo -> one Docker image -> one Docker Compose service.

It clones a repo, builds it when the target branch changes, tags the image with a stable tag plus the commit SHA, and asks Docker Compose to recreate the managed service.

## Quick start

See `compose.example.yml`.

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

## Required configuration

```env
GIT_URL=git@github.com:your-org/private-app.git
GIT_BRANCH=main
GIT_AUTH_MODE=none|ssh
GIT_SSH_KEY_PATH=/run/secrets/git_ssh_key

IMAGE_NAME=private-app:live
DOCKERFILE=Dockerfile
BUILD_CONTEXT=.

COMPOSE_FILE=/compose/docker-compose.yml
COMPOSE_PROJECT_NAME=my-stack
COMPOSE_SERVICE=app
COMPOSE_PROFILE=app

CHECK_INTERVAL=30
KEEP_BUILDS=3
DEPLOY_ON_START=true
DATA_DIR=/data
LOG_LEVEL=info
```

## How it redeploys

After a successful build, miniploy runs:

```bash
docker compose \
  -f "$COMPOSE_FILE" \
  -p "$COMPOSE_PROJECT_NAME" \
  --profile "$COMPOSE_PROFILE" \
  up -d --no-deps --force-recreate "$COMPOSE_SERVICE"
```

Use `REDEPLOY_ARGS` to override `--no-deps --force-recreate`.

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
