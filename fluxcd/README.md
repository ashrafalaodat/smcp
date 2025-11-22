# FluxCD Helm Chart

This chart vendors the upstream [`install.yaml`](https://github.com/fluxcd/flux2/releases) bundle so that you can deploy Flux with `helm upgrade --install` instead of relying on the Flux CLI.  The manifest that ships today is based on Flux **v2.7.3** and includes all controllers plus the official CRDs.

## Usage

```bash
helm upgrade --install fluxcd ./fluxcd \
  --namespace flux-system \
  --create-namespace
```

Key values in `values.yaml`:

- `targetNamespace`: Namespace for every Flux component (defaults to `flux-system`).
- `createNamespace`: Set to `false` if the namespace already exists.
- `installCRDs`: Disable if you manage CRDs via another mechanism.
- `images.*`: Override repositories, tags, digests, or pull policies per controller.

After installation you can inspect the rollout with:

```bash
kubectl -n flux-system get pods
```

## Updating the vendored manifest

1. Find the Flux release you want to ship and download its `install.yaml`:
   ```bash
   curl -L -o /tmp/install.yaml https://github.com/fluxcd/flux2/releases/download/<version>/install.yaml
   ```
2. Replace `files/install.yaml` with the downloaded file.
3. Re-run the helper in `files/install.yaml` (search-and-replace `flux-system` with `{{ include "fluxcd.namespace" . }}` and update the image tags if they changed).
4. Adjust `values.yaml` and `Chart.yaml` (especially `appVersion`) to match the new release.
5. Run `helm lint fluxcd` before publishing.

Because the upstream manifest is large, the chart deliberately keeps templating minimal—only namespaces, CRD toggling, and image fields are parameterised to stay close to the supported Flux footprint.
