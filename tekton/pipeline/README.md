# Image build pipeline

This folder contains Tekton resources that clone `http://gitea-gitea.gitea.svc.cluster.local:3000/gitea-admin/math.git` (branch `main`) and push a new image into the in-cluster registry (`oci-registry-distribution.oci.svc.cluster.local:5000`) whenever a push lands on `main`.

## Files

- `pipeline.yaml` – defines the `git-image-build` pipeline plus the `tekton-image-builder` service account (all scoped to the `oci` namespace).
- `triggers.yaml` – provides a Gitea webhook listener (TriggerBinding + TriggerTemplate + EventListener) so pushes to `main` automatically create PipelineRuns.

## Prerequisites

1. Tekton Pipelines + Triggers are already installed in the cluster (see `tekton/charts/tekton`).
2. The namespace `oci` exists (Tekton resources in this folder live there alongside the OCI registry secret).
3. Secret `oci-registry-distribution-dockerconfig` already exists in the `oci` namespace and contains a `.dockerconfigjson` entry (e.g., create it with `kubectl create secret docker-registry`). The pipeline mounts this secret read-only and copies it into `/tekton/home/.docker/config.json` before running Kaniko.
4. ConfigMap `knative-dockerfiles` exists in `oci` with a key `Dockerfile-go`. If the repo lacks the specified Dockerfile, the pipeline copies this fallback into the workspace and builds a static Go binary (`./func`) before invoking Kaniko.
5. The default Tekton pod has enough scratch space (the `source` workspace uses an `emptyDir`).
4. Ensure the Gitea repository `gitea-admin/math` is reachable from the cluster via HTTP.

## Deploy

Apply the manifests into the `oci` namespace (the YAML already pins the namespace):

```bash
kubectl apply -f tekton/pipeline/pipeline.yaml
kubectl apply -f tekton/pipeline/triggers.yaml
```

After the `EventListener` (named `gitea-image-listener`) rolls out, expose it with port-forwarding while you create the Gitea webhook (Gitea needs a reachable endpoint, so use something like `ngrok` or `cloudflared tunnel`):

```bash
kubectl -n oci port-forward svc/el-gitea-image-listener 8080:8080
```

In your Gitea repo settings (`gitea-admin/math`), create a webhook:

- **Payload URL:** `https://<public-forwarder>/` (Tekton listener listens on `/`)
- **Content type:** `application/json`
- **Secret:** optional (no signature validation is enabled by default—add it via CEL if desired)
- **Events:** `Just the push event`
- **Branch filter:** optional. The EventListener already filters for `refs/heads/main`, but you can add UI-side filtering if you want an extra guardrail.

## Customizing the build

- **Registry host:** If your registry runs elsewhere, edit `pipeline.yaml` (the `registry_host` param + `REGISTRY_HOST` env) and `triggers.yaml` to match.
- **Dockerfile/context:** Adjust `pipeline.yaml` defaults if `math.git` stores its Dockerfile somewhere else. When no Dockerfile is present, the pipeline automatically injects `Dockerfile-go` from the `knative-dockerfiles` ConfigMap.
- **Image tag format:** The EventListener sets `image_tag` to `<short-sha>-<timestamp>` where `timestamp` is derived from `head_commit.timestamp` (`YYYYMMDDHHMMSS`). Update `triggers.yaml` if you want a different convention.
- **Branch filter:** The CEL interceptor currently filters `refs/heads/main`. Update the expression in `triggers.yaml` if you want to watch more refs.

## Manual test

If you want to trigger a build without GitHub, craft a `PipelineRun` manually:

```bash
cat <<'YAML' | kubectl apply -f -
apiVersion: tekton.dev/v1
kind: PipelineRun
metadata:
  name: git-image-build-manual
  namespace: oci
spec:
  taskRunTemplate:
    serviceAccountName: tekton-image-builder
  pipelineRef:
    name: git-image-build
  params:
    - name: repo_name
      value: math
    - name: image_tag
      value: deadbee
  workspaces:
    - name: source
      emptyDir: {}
    - name: dockerconfig
      secret:
        secretName: oci-registry-distribution-dockerconfig
YAML
```

Watch the run:

```bash
kubectl -n oci get pipelineruns -w
```

## Clean up

```bash
kubectl delete -f tekton/pipeline/triggers.yaml
kubectl delete -f tekton/pipeline/pipeline.yaml
```
