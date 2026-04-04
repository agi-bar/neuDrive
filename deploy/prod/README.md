# Production Deploy

Recommended server layout:

```text
~/apps/agenthub/
  bin/
    deploy-main
    status
  logs/
  repo/
```

`repo/` is a clean git checkout of `git@github.com:agi-bar/agenthub.git`.

`bin/deploy-main` is a thin wrapper that:

1. moves into `repo/`
2. updates to the latest `origin/main`
3. runs `deploy/prod/deploy.sh`
4. writes a timestamped log file under `logs/`

`deploy/prod/deploy.sh` builds the Docker image directly inside the minikube
Docker daemon, applies the Kubernetes manifests, updates the deployment image,
waits for rollout, and verifies `https://agenthub.agi.bar/api/health`.

Useful commands from the server:

```bash
~/apps/agenthub/bin/deploy-main
~/apps/agenthub/bin/status
```
