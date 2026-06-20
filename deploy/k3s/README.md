# Deploying the NTunl host on k3s

These manifests run the NTunl **host** (tunnel server + public proxy + admin
portal) on k3s. They rely on two things k3s bundles by default: the **local-path**
storage provisioner (for the SQLite volume) and **Traefik** (ingress).

## Layout

| File              | Purpose                                                        |
|-------------------|----------------------------------------------------------------|
| `namespace.yaml`  | `ntunl` namespace                                              |
| `configmap.yaml`  | `host.json` (set your domain here)                             |
| `secret.yaml`     | seeds the first admin (`NTUNL_ADMIN_USER/PASSWORD`)           |
| `pvc.yaml`        | 1Gi RWO volume mounted at `/data` (SQLite DB + cert)          |
| `deployment.yaml` | the host pod (1 replica, `Recreate`)                          |
| `service.yaml`    | ClusterIP over ports 8001/9200/8002 (+ optional LoadBalancer) |
| `ingress.yaml`    | Traefik rules for portal / tunnel / wildcard proxy           |
| `kustomization.yaml` | ties it together; override the image here                 |

## Before you apply

1. **Build & push the image** from the repo root, then point the manifests at it:
   ```bash
   docker build -f build/Dockerfile.host -t <registry>/ntunl-host:<tag> .
   docker push <registry>/ntunl-host:<tag>
   ```
   Set it via `kustomization.yaml` `images:` or edit `deployment.yaml`.
2. **Set your domain** — replace `example.com` in `configmap.yaml` (clientDomain)
   and `ingress.yaml` (the three hosts).
3. **Set the admin password** in `secret.yaml` (or delete the Secret to fall back
   to `admin`/`admin`). It only applies on first run with an empty database.
4. **DNS**: one **DNS-only** wildcard A record covers everything (it also matches
   `portal.`/`tunnel.`):
   ```
   *.<domain>   A   <traefik external IP>   # kubectl -n kube-system get svc traefik -o wide
   ```
   A records can't specify a port, so clients reach 443/80 and Traefik routes by
   hostname to ports 8002/8001/9200.

## Apply

```bash
kubectl apply -k deploy/k3s
# or, without kustomize:
kubectl apply -f deploy/k3s
```

Watch it come up:

```bash
kubectl -n ntunl rollout status deploy/ntunl-host
kubectl -n ntunl logs deploy/ntunl-host
```

## Use it

- Portal: `https://portal.<domain>` — sign in, change the admin password, create
  users (Admin → Users). Subdomains are assigned automatically on connect.
- Client: `ntunl-client login -portal https://portal.<domain>` then
  `ntunl-client run`. Point the client's `ntunlAddress` at `tunnel.<domain>:443`
  with `sslEnabled: true` (TLS terminated at the ingress).

## TLS / HTTPS

Public URLs are `https://<sub>.<domain>` and clients use `wss://`, so you need a
**wildcard certificate** at the ingress (the `tls:` block in `ingress.yaml` already
references `ntunl-wildcard-tls`). Issue it with cert-manager + Cloudflare DNS-01 —
see [`cert-manager/`](./cert-manager). Until the cert is issued, traffic is plain
HTTP and clients must use `sslEnabled: false` against `<domain>:80`.

## Notes

- Keep `replicas: 1` — SQLite is single-writer and the volume is `ReadWriteOnce`.
- `tunnel` and `portal` are routed to the tunnel/portal ports, not the proxy, so
  a client requesting either name as its subdomain won't be reachable.
- To scale storage, grow the PVC `resources.requests.storage` (local-path
  supports expansion on most setups).
