#!/usr/bin/env bash
set -euo pipefail

REG_NAME=${REG_NAME:-local-registry}
REG_PORT=${REG_PORT:-5000}
CLUSTER_NAME=${CLUSTER_NAME:-nvkind-kgwkg}
ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
CONFIG_FILE=${CONFIG_FILE:-"${ROOT_DIR}/kind-local-registry.yaml"}
CERT_ROOT=${CERT_DIR:-"${HOME}/.kind/certs.d"}
CERT_MIRROR_DIR="${CERT_ROOT%/}/${REG_NAME}:${REG_PORT}"

command -v kind >/dev/null 2>&1 || { echo "kind command not found" >&2; exit 1; }
command -v docker >/dev/null 2>&1 || { echo "docker command not found" >&2; exit 1; }

mkdir -p "${CERT_MIRROR_DIR}"
cat > "${CERT_MIRROR_DIR}/hosts.toml" <<HOSTS
server = "http://${REG_NAME}:${REG_PORT}"

[host."http://${REG_NAME}:${REG_PORT}"]
  capabilities = ["pull", "resolve", "push"]
  skip_verify = true
HOSTS

cat > "${CONFIG_FILE}" <<CFG
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri"]
    [plugins."io.containerd.grpc.v1.cri".registry]
      config_path = "/etc/containerd/certs.d"
nodes:
- role: control-plane
  extraMounts:
  - hostPath: "${CERT_ROOT}"
    containerPath: "/etc/containerd/certs.d"
- role: worker
  extraMounts:
  - hostPath: "${CERT_ROOT}"
    containerPath: "/etc/containerd/certs.d"
CFG

kind create cluster --name "${CLUSTER_NAME}" --config "${CONFIG_FILE}"

docker network connect kind "${REG_NAME}" >/dev/null 2>&1 || true

echo "Cluster '${CLUSTER_NAME}' ready with registry mirror ${REG_NAME}:${REG_PORT}."
