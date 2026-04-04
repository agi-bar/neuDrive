# Production Deploy

Recommended server layout:

```text
~/apps/agenthub/
  bin/
    deploy-main
    status
  config/
    agenthub.env
  k8s/
  logs/
  repo/
```

`repo/` is a clean git checkout of `git@github.com:agi-bar/agenthub.git`.
`config/agenthub.env` is copied from `agenthub.env.example` and holds the real
deployment settings and secrets.

`bin/deploy-main` is a thin wrapper that:

1. moves into `repo/`
2. updates to the latest `origin/main`
3. runs `deploy/prod/deploy.sh`
4. writes a timestamped log file under `logs/`

`deploy/prod/deploy.sh` builds the Docker image directly inside the minikube
Docker daemon, syncs the tracked manifests into `k8s/`, creates or updates the
runtime secrets/config from `config/agenthub.env`, applies the Kubernetes
manifests, updates the deployment image, waits for rollout, and verifies the
public healthcheck.

Useful commands from the server:

```bash
cp ~/apps/agenthub/repo/agenthub.env.example ~/apps/agenthub/config/agenthub.env
vim ~/apps/agenthub/config/agenthub.env
~/apps/agenthub/bin/deploy-main
~/apps/agenthub/bin/status
```
