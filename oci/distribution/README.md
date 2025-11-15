# distribution Helm chart

This chart deploys the upstream [distribution](https://github.com/distribution/distribution) reference implementation of the OCI/Docker registry (`registry:2`) on Kubernetes. It wraps the upstream configuration file and exposes the common knobs (storage, TLS termination via ingress, authentication, metrics, etc.) so you can run a private registry or embed it into a larger platform.

## Requirements

- Kubernetes 1.24+
- Helm 3.9+
- A default `StorageClass` (or provide `persistence.existingClaim`)
- (Optional) cert-manager/Ingress controller for HTTPS and Prometheus CRDs for the ServiceMonitor resource

## Install

```bash
helm upgrade --install my-registry oci/distribution \
  --namespace registry --create-namespace
```

Port-forward to test:

```bash
kubectl -n registry port-forward svc/my-registry-distribution 5000:5000
curl -I http://127.0.0.1:5000/v2/
```

## Key configuration

| Section | Description |
|---------|-------------|
| `image.*` | Registry image reference (defaults to `registry:3.0.0`). |
| `persistence.*` | PVC settings for `/var/lib/registry`. Disable for ephemeral installs or point to an existing claim. |
| `registryConfig.*` | Controls how the upstream `config.yml` is provided. The chart renders the string in `registryConfig.data` through Helm templates so you can reference any value. Use `registryConfig.existingSecret` to mount your own file instead. |
| `httpSecret.*` | Provides the `REGISTRY_HTTP_SECRET` env var that signs tokens. Helm will reference an existing secret/key or take the literal value. |
| `auth.htpasswd.*` | Built-in basic auth ships with user `oci` / `JQdMvN43Qkfo9IoFOTHW94`. The Secret also stores the username/password fields for retrieval. Replace the plaintext + bcrypt hash in `values.yaml`, add more `entries`, or point to `existingSecret` for production. |
| `ingress.*` | Standard Kubernetes ingress options. Remember to configure TLS at the ingress/controller layer. |
| `metrics.*` and `serviceMonitor.*` | Adds an optional debug listener/ServiceMonitor on `metrics.port`. Make sure your `registryConfig.data` turns on the [`http.debug` section](https://distribution.github.io/distribution/about/configuration/) so the process actually serves metrics. |
| `extraEnv`, `extraVolumeMounts`, `extraVolumes` | Escape hatches for custom storage drivers, cloud credentials, and object storage configuration values. |

### Example: enable basic auth + ingress

```yaml
auth:
  htpasswd:
    enabled: true
    entries:
      - username: demo
        passwordHash: "$2y$05$Dr0pInYourOwnBcryptHashHere"
ingress:
  enabled: true
  className: nginx
  hosts:
    - host: registry.example.com
      paths:
        - path: /
          pathType: Prefix
httpSecret:
  value: "change-me"
```

### Example: use a custom config file

If you already manage the upstream `config.yml`, place it in a secret:

```bash
kubectl create secret generic registry-config \
  --from-file=config.yml=./config.yml -n registry
```

and update values:

```yaml
registryConfig:
  existingSecret: registry-config
  secretKey: config.yml
```

## Development

Validate template rendering before shipping values:

```bash
helm lint oci/distribution
helm template my-registry oci/distribution --namespace registry
```

If you enable Prometheus scraping, make sure the CRDs from kube-prometheus-stack (or equivalent) exist before installing the chart.
