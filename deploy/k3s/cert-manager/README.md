# Wildcard TLS for the NTunl host (direct-to-IP, no Cloudflare proxy)

When you route `*.<domain>` straight at the k3s ingress IP with a **DNS-only**
wildcard A record (instead of through a Cloudflare proxied tunnel), Cloudflare's
edge certificate no longer applies — you must terminate TLS yourself. These
manifests issue a Let's Encrypt **wildcard** cert via cert-manager + Cloudflare
DNS-01 and hand it to the Ingress.

These are intentionally **not** part of the top-level `kustomization.yaml`: they
depend on cert-manager's CRDs, which must be installed first.

## Steps

1. **Install cert-manager** (one-time, cluster-wide):
   ```bash
   kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
   ```

2. **DNS** — point a wildcard at your ingress IP, **DNS-only / grey cloud**
   (proxied wildcards are Enterprise-only and won't work here):
   ```
   *.example.com   A   <traefik external IP>
   ```
   Find the IP: `kubectl -n kube-system get svc traefik -o wide`.

3. **Cloudflare API token** — create one (Zone:DNS:Edit + Zone:Zone:Read on your
   zone), put it in `cloudflare-api-token-secret.yaml`, and apply it into the
   `cert-manager` namespace.

4. **Edit the domain/email** in `clusterissuer.yaml` and `certificate.yaml`
   (replace `example.com` and the email). Tip: use the Let's Encrypt **staging**
   server first to avoid rate limits, then switch to prod.

5. **Apply**:
   ```bash
   kubectl apply -f deploy/k3s/cert-manager/cloudflare-api-token-secret.yaml
   kubectl apply -f deploy/k3s/cert-manager/clusterissuer.yaml
   kubectl apply -f deploy/k3s/cert-manager/certificate.yaml
   ```

6. **Verify** the cert was issued into the `ntunl-wildcard-tls` secret that the
   Ingress references:
   ```bash
   kubectl -n ntunl get certificate ntunl-wildcard
   kubectl -n ntunl describe certificate ntunl-wildcard   # watch for "Issued"
   ```

The Ingress (`../ingress.yaml`) already references `secretName: ntunl-wildcard-tls`,
so once the cert is `Ready`, `https://<sub>.example.com` and `wss://tunnel...`
serve a valid certificate.

## Client config for this path

Because TLS is now terminated at Traefik on 443:

```jsonc
{
  "sslEnabled": true,
  "allowInvalidCertificates": false,   // real Let's Encrypt cert
  "ntunlAddress": "tunnel.example.com:443"
}
```

## Trade-off vs. Cloudflare Tunnel

Direct-to-IP exposes your node's public IP and you manage TLS yourself, but it's
the only way to get wildcard/dynamic subdomains without a Cloudflare Enterprise
plan. The portal and tunnel-handshake hostnames could still go through a
Cloudflare proxied tunnel if you prefer to hide those two — but the wildcard
proxy must be direct.
