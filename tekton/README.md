# Tekton on kind via Helm

This directory holds the `tekton` chart, which wraps the upstream Tekton release manifests so you can install Pipelines/Triggers/Dashboard into any cluster (including the local `kind` cluster that lives under this repo).

## Folder layout

```
tekton/
  README.md          # this file
  charts/
    tekton/          # Helm chart for installing Tekton components
```

## Usage

1. Point your shell at the repo root and make sure `kubectl` is talking to your `kind` cluster (e.g. `kind get clusters`, `kubectl config use-context kind-kind`).
2. Install (or upgrade) Tekton with Helm:
   ```bash
   helm upgrade --install tekton ./tekton/charts/tekton \
     --namespace tekton \
     --create-namespace
   ```
3. Watch the hook jobs complete:
   ```bash
   kubectl -n tekton get jobs -w
   ```
4. Confirm the Tekton deployments in the `tekton-pipelines` namespace:
   ```bash
   kubectl -n tekton-pipelines get pods
   ```

## Configuration knobs

The chart streams official release bundles directly from Google Cloud Storage. You can pin versions or enable/disable components in `values.yaml`. Example override:

```yaml
components:
  pipelines:
    url: https://storage.googleapis.com/tekton-releases/pipeline/previous/v0.57.0/release.yaml
  dashboard:
    enabled: true
```

Apply with:

```bash
helm upgrade --install tekton ./tekton/charts/tekton \
  --namespace tekton \
  --create-namespace \
  -f my-tekton-values.yaml
```

The hook jobs run with a dedicated service account plus a high-privilege ClusterRole because Tekton distributes CRDs, cluster-wide RBAC, and webhooks. If you prefer to use pre-created credentials, set `serviceAccount.create=false` and point `serviceAccount.name` at your own account, then wire your own RBAC.

## Uninstall

`helm uninstall tekton --namespace tekton`

The chart's pre-delete hooks will stream the same upstream bundles through `kubectl delete --ignore-not-found`, so Tekton CRDs, RBAC, and deployments are cleaned up before Helm removes the helper jobs.
