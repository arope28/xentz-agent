# Client onboarding SOP (v1)

Standard operating procedure from contract/sign-up to first successful backup. The portal is **admin-oriented** in v1; clients typically receive instructions from your team rather than self-signup inside the product.

## Before the install call

1. **Tenant / billing:** Create or confirm tenant in your CRM and Stripe (if used). Ensure Stripe metadata includes `tenant_id` matching the control plane tenant row (see control plane `docs/STRIPE_BILLING_SETUP.md`).
2. **Invite / install token:** Mint an install token or active invite code on the control plane; choose expiry appropriate to your policy.
3. **Repository:** Confirm `XENTZ_REPO_BASE` and storage backend can accept new device paths for this tenant.

## Client touchpoint (email or call script)

Send:

- Control plane public URL (`XENTZ_CONTROL_BASE`).
- One-time **install token** (treat as secret; do not post in public tickets).
- Suggested install command (adjust paths and time):

  ```text
  xentz-agent install --token <TOKEN> --server https://<control-host> --daily-at 02:00 --include "<path-to-backup>"
  ```

- Link or attach: **restic** install link for their OS ([restic.net](https://restic.net)) if not using your bundled installer.
- **Who to contact** if install fails (support email/slack) and reminder to run `xentz-agent diagnostics` if asked.

## After install

1. In portal (`/ui/devices`), confirm new device, hostname, last seen.
2. Ask client to run one manual backup if the schedule is far off:  
   `xentz-agent backup`
3. Confirm run appears in device detail and metrics; fix include paths if backup size is zero unexpectedly.

## v1 scope note (self-service)

Full self-service signup inside the portal is **not** required for v1 per product checklist. If you add a narrow later slice (e.g. read-only device list for tenant users), document it separately and harden auth.

## Related documents

- Phase 0 smoke: [PHASE0_SMOKE_CHECKLIST.md](PHASE0_SMOKE_CHECKLIST.md)
- Recovery for clients: [CLIENT_RECOVERY_GUIDE.md](CLIENT_RECOVERY_GUIDE.md)
