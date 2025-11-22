# Qdrant Helm Chart

This chart provisions a GPU-ready Qdrant StatefulSet with persistent storage so the MCP registry has a dedicated ANN backend. Key defaults:

- GPU usage is **disabled by default**; set `gpu.enabled=true` to request `nvidia.com/gpu` resources (count configurable via `gpu.count`).
- Exposes both HTTP (6333) and gRPC (6334) services.
- Mounts `/qdrant/storage` on a 20Gi PVC so indexes persist across restarts.

Override `values.yaml` to adjust storage class, GPU sizing, or replica counts (Qdrant clustering works by fronting multiple StatefulSets with sharding/replication if needed).
