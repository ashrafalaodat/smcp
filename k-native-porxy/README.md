# Knative Localhost Proxy

Scripts for exposing every Knative Service as `http://{name}.{namespace}.localhost`.

## Usage

```bash
./start.sh
```

The script:

- Detects the Kourier ingress Node IP and NodePort from your cluster.
- Launches (or replaces) a `caddy:2` container on the host network with the correct upstream.
- Proxies any host `*.localhost` requests to Knative, so `curl http://ads.default.localhost` works out of the box.

Stop the proxy with:

```bash
./stop.sh
```

You can override behaviour with environment variables:

- `CONTAINER_NAME` (default: `knative-localhost-proxy`)
- `CADDY_IMAGE` (default: `caddy:2`)
- `KOURIER_NODE_IP`
- `KOURIER_NODE_PORT`
