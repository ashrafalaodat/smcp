# Image build pipeline

This folder contains Tekton resources that build and push a container image every time a push hits the `main` branch of your GitHub repository.

## Files

- `pipeline.yaml` – defines the `git-image-build` pipeline plus the `tekton-image-builder` service account that Kaniko uses.
- `triggers.yaml` – provides a GitHub webhook listener (TriggerBinding + TriggerTemplate + EventListener) so pushes to `main` automatically create PipelineRuns.

## Prerequisites

1. Tekton Pipelines + Triggers are already installed in the cluster (see `tekton/charts/tekton`).
2. A registry secret named `registry-credentials` exists in the `tekton-pipelines` namespace:
   ```bash
   kubectl create secret docker-registry registry-credentials \
     --namespace tekton-pipelines \
     --docker-server=ghcr.io \
     --docker-username=<user> \
     --docker-password=<token> \
     --docker-email=<email>
   ```
3. A GitHub webhook secret named `github-webhook-secret` with key `secretToken` exists in `tekton-pipelines`:
   ```bash
   kubectl create secret generic github-webhook-secret \
     --namespace tekton-pipelines \
     --from-literal=secretToken=<random-string>
   ```
4. (Optional) Install the [Tekton git-clone](https://hub.tekton.dev/tekton/task/git-clone) task if you prefer catalog tasks. This sample vendors its own inline clone/build steps, so no extra catalog tasks are required.

## Deploy

Apply the manifests into the `tekton-pipelines` namespace (the same namespace where the Tekton controllers live):

```bash
kubectl apply -f tekton/pipeline/pipeline.yaml
kubectl apply -f tekton/pipeline/triggers.yaml
```

After the `EventListener` rolls out, expose it with port-forwarding while you create the GitHub webhook (GitHub needs a reachable HTTPS endpoint, so use something like `ngrok` or `cloudflared tunnel`):

```bash
kubectl -n tekton-pipelines port-forward svc/el-github-image-listener 8080:8080
```

In your GitHub repo settings, create a webhook:

- **Payload URL:** `https://<public-forwarder>/` (Tekton listener listens on `/`)
- **Content type:** `application/json`
- **Secret:** the same value you stored in `github-webhook-secret`
- **Events:** `Just the push event`

## Customizing the build

- **Image repository:** Update `triggers.yaml` → `EventListener` → `params.imageRepository` with your registry/repo (default `ghcr.io/example-org/sample-app`).
- **Dockerfile/context:** Change the `dockerfile` and `contextPath` params in the same section if your repo uses a different layout.
- **Branch filter:** The CEL interceptor currently filters `refs/heads/main`. Adjust `value` to target other branches or tags.

## Manual test

If you want to trigger a build without GitHub, craft a `PipelineRun` manually:

```bash
cat <<'YAML' | kubectl apply -f -
apiVersion: tekton.dev/v1
kind: PipelineRun
metadata:
  name: git-image-build-manual
  namespace: tekton-pipelines
spec:
  serviceAccountName: tekton-image-builder
  pipelineRef:
    name: git-image-build
  params:
    - name: repo_url
      value: https://github.com/example/repo.git
    - name: revision
      value: main
    - name: image_url
      value: ghcr.io/example-org/sample-app:test
    - name: dockerfile
      value: Dockerfile
    - name: context_path
      value: .
  workspaces:
    - name: source
      emptyDir: {}
    - name: dockerconfig
      secret:
        secretName: registry-credentials
YAML
```

Watch the run:

```bash
kubectl -n tekton-pipelines get pipelineruns -w
```

## Clean up

```bash
kubectl delete -f tekton/pipeline/triggers.yaml
kubectl delete -f tekton/pipeline/pipeline.yaml
```
