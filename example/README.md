# Self-contained nginx example

This example deploys a custom nginx image from this repository using miniploy.

## Files

- `Dockerfile` builds a small nginx image and bakes in a “Hello from miniploy” page.
- `compose.yaml` runs miniploy and the managed nginx service.

## Run it

From the repo root:

```bash
docker compose -f example/compose.yaml up -d miniploy
```

Miniploy will pull `ghcr.io/itsamenathan/miniploy:latest`, clone `https://gitlab.com/itsamenathan/miniploy.git`, build `example/Dockerfile`, tag it as `miniploy-nginx-example:live`, then recreate the `nginx` service.

Open http://localhost:8080 after the deployment finishes.

Because miniploy deploys Git commits, push changes before testing updates from GitLab.

If you are testing from a branch other than `main`, pass it explicitly:

```bash
GIT_BRANCH=$(git branch --show-current) docker compose -f example/compose.yaml up -d miniploy
```

View miniploy logs:

```bash
docker compose -f example/compose.yaml logs -f miniploy
```

Control the deployed nginx service:

```bash
docker compose -f example/compose.yaml exec miniploy miniployctl help
docker compose -f example/compose.yaml exec miniploy miniployctl status
docker compose -f example/compose.yaml exec miniploy miniployctl logs -f
docker compose -f example/compose.yaml exec miniploy miniployctl redeploy
docker compose -f example/compose.yaml exec miniploy miniployctl rebuild
```

Stop and remove the example stack:

```bash
docker compose -f example/compose.yaml down --volumes
```
