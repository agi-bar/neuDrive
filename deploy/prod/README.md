# Production Deploy

Recommended server layout:

```text
~/apps/neudrive/
  bin/
    deploy-main
    status
  config/
    neudrive.env
  k8s/
  logs/
  repo/
```

`repo/` is a clean git checkout of `git@github.com:agi-bar/neudrive.git`.
`config/neudrive.env` is copied from `neudrive.env.example` and holds the real
deployment settings and secrets.

`bin/deploy-main` is a thin wrapper that:

1. moves into `repo/`
2. updates to the latest `origin/main`
3. runs `deploy/prod/deploy.sh`
4. writes a timestamped log file under `logs/`

`deploy/prod/deploy.sh` builds the Docker image directly inside the minikube
Docker daemon, syncs the tracked manifests into `k8s/`, creates or updates the
runtime secrets/config from `config/neudrive.env`, applies the Kubernetes
manifests, updates the deployment image, waits for rollout, and verifies the
public healthcheck.

Useful commands from the server:

```bash
cp ~/apps/neudrive/repo/neudrive.env.example ~/apps/neudrive/config/neudrive.env
vim ~/apps/neudrive/config/neudrive.env
~/apps/neudrive/bin/deploy-main
~/apps/neudrive/bin/status
```

## Bundle Sync Prod-like 验收

上线或大版本同步改动后，建议在这台 prod-like 机器上额外跑一次 Bundle Sync 验收。

推荐顺序：

1. 部署最新 `origin/main`
2. 在管理后台生成一个 `both` Sync Token
3. 用匿名 fixture 跑 `export -> preview -> push -> pull -> diff`
4. 再用一套真实 `.ndrvz` 跑同样流程
5. 单独验证一次 archive `resume`
6. 单独验证一次 `mirror` 删除边界

完整 Runbook 见：

- [`docs/sync-prodlike-acceptance.md`](../../docs/sync-prodlike-acceptance.md)
- [`docs/sync.md`](../../docs/sync.md)
