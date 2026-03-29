# Owner security checklist — draft response

Use this as a starting point for email or Slack. It reflects the current **xentz-agent** implementation and related control-plane design; adjust if production differs.

---

Hi — answers below on the pre-release security questions for the backup agent.

### Least privilege

**Partially yes.** Default install is **user** mode (no ongoing admin). **System** mode uses launchd/systemd/Windows service and usually needs elevated install once. We recommend user mode for workstations and system mode only when policy requires it.

### Credentials

**Strong enrollment, long-lived device keys.** No hardcoded API keys in the agent. Install uses a **short-lived, single-use** enrollment token; the server issues a **per-device API key** stored in OS secret storage (Keychain / DPAPI / libsecret, with a restricted Linux file fallback). Restic repo passwords are held the same way after enrollment.

**Gaps to be aware of:** the device API key is **long-lived** (rotation is mainly **revoke + re-enroll**, not automatic periodic rotation). Passing `--token` on the command line can leave traces in shell history. Until migration completes, `device_api_key` may still appear in `config.json` as a fallback.

### TLS and certificate validation

**Standard verification, no pinning.** The agent uses Go’s default HTTPS client — **no** “ignore certificate errors” mode in code. **However**, the agent still allows an `http://` control-plane URL; production should **require HTTPS** by policy (or we can add a strict mode later). There is **no certificate pinning**; we rely on the OS trust store.

### Backup encryption and keys

**Yes — Restic encrypts repository data.** The encryption key material is the **restic repository password**, stored locally in the OS secret store when possible. The control plane stores the password encrypted server-side. Logs redact passwords, API keys, and tokens.

### Tampering (signed binary, protected config)

**Limited in the agent itself.** `config.json` is written with `0600` permissions; the local status UI redacts secrets. There is **no** in-process binary signature or integrity check — rely on **signed installers / distribution channel** (notarization, vendor packages, etc.) for supply-chain assurance.

### Updates — signed and verified?

**Not inside the agent today.** `upgrade --binary` replaces the executable with **no** cryptographic verification. The standalone GitHub downloader uses HTTPS only. **Recommendation:** verify releases with published checksums/signatures (e.g. cosign) until we wire verification in.

### Security / audit logging

**Partial.** Structured JSON logs with redaction ship to the control plane for backup/retention reporting. **Server-side** audit (admin config changes, revoke, etc.) belongs on the control plane. The **local web UI** returns 401 on bad tokens but does **not** yet write a dedicated audit log for failed local auth or every restore action.

### Ransomware resilience

**Snapshots yes; immutability is operational.** Restic gives **versioned snapshots**. **Object lock / immutable storage / delayed delete** are **backend and ops** choices (e.g. S3 Object Lock), not enforced by the agent. Retention with **prune** can permanently remove old snapshots — that should be a deliberate policy.

### Validating server-supplied config

**Yes — consistently after a recent hardening change.** Both **GET** and **PUT** config responses run the same checks: kill-switch, tenant/device identity, required fields, limits on include/exclude list size, and path sanity (length, no null bytes). Restic backup also uses `--` before paths to reduce flag injection.

### Local secrets

**OS stores preferred; fallbacks documented.** Keychain / DPAPI / libsecret where available; Linux file fallback under the config dir with `0600`. Local UI uses a random token in `local-ui.token` with `0600`.

### Restores

**Localhost + secret token for the browser UI.** CLI restore runs as the logged-in user with access to config/secrets — same threat model as other endpoint backup tools. Anyone with the **local UI token** can drive restores; tokens in browser URLs are a documented usability tradeoff.

### Security testing so far

**Functional smoke tests** (enroll, backup, kill-switch), not a formal adversarial program. **Recommendation before GA:** short internal pass (TLS, revoked keys, malicious upgrade binary, local UI abuse cases) and optional **external pen test** if compliance requires it.

---

### Summary table

| Topic | Status |
| ----- | ------ |
| Least privilege | User default; system mode = elevated install |
| Credentials | Strong storage; long-lived device key; rotation = revoke/re-enroll |
| TLS / certs | Default verify; `http` URL still allowed; no pinning |
| Backup encryption | Restic + password in secret store |
| Tamper-resistant agent | Config perms; no self-check on binary |
| Signed updates | No in-agent verification yet |
| Audit logs | Partial; server audit for admin; weak local UI audit trail |
| Ransomware | Snapshots yes; immutability = storage/backend |
| Server input validation | Aligned on GET and PUT config paths |
| Local secrets | OS stores + documented fallbacks |
| Restore security | Local token + localhost; host compromise = high risk |
| Formal security testing | Smoke-level; no dedicated MITM/tamper suite |

---

**Bottom line:** The agent is in good shape on **secrets handling**, **Restic encryption**, **no cert bypass**, **server config validation**, and **kill-switch behavior**. Before release, we should be explicit with customers about **HTTPS-only deployment**, **no signed auto-updates yet**, **audit logging gaps on the endpoint**, and **immutable backup storage** as an ops/backend choice.
