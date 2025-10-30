# Kind + Local Registry bootstrap

This folder contains a helper script that wires every new kind cluster to the local insecure Docker registry (`local-registry:5000`).

## Files
- `setup-kind-local-registry.sh` – prepares the containerd mirror config, writes `kind-local-registry.yaml`, creates the cluster, and connects the registry container to the kind network.
- `kind-local-registry.yaml` – generated on each run; you can inspect or reuse it once the script finishes.

## Usage
```bash
cd kind
./setup-kind-local-registry.sh
```

Environment variables let you tweak the defaults:
- `CLUSTER_NAME` – target kind cluster name (default `nvkind-kgwkg`).
- `REG_NAME` / `REG_PORT` – local registry host and port (default `local-registry:5000`).
- `CERT_DIR` – base directory for the containerd registry mirror config (default `$HOME/.kind/certs.d`; the script creates `<registry>:<port>/hosts.toml` inside it).
- `CONFIG_FILE` – path for the generated kind config (default `kind-local-registry.yaml` beside the script).

Run the script before any `helm install`/`kubectl apply` so the nodes trust the registry from the outset.
