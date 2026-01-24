# Recovery / Relink (Device Identity)

This agent separates **durable enrollment identity** from the policy `config.json` so an upgrade/reinstall (or accidental config deletion) doesn’t silently strand prior backups.

## What is durable identity?

The agent stores:

- **Enrollment identity** in `identity.json` under `STATE_DIR` (mode-dependent)
- **Device/Principal API key** and **Restic password** in the OS secret store (`internal/secretstore`)

This means the agent can often recover even if `config.json` is lost.

## Scenario A: `config.json` deleted, but identity + secrets remain

1. Ensure the service is still installed (or run manually): `xentz-agent backup`
2. The agent will:
   - Load `identity.json` (server URL, tenant/device id)
   - Load API key from secret store
   - Fetch policy config from the control plane and continue

No portal action is required.

## Scenario B: enrollment secrets lost (or a restored machine needs relink)

You need a **portal-minted recovery token**.

1. In the portal UI, open the device and find **Backup Principals**.
2. Click **Generate token** (this shows a one-time token and expiry).
3. On the machine, run:

```bash
xentz-agent recover --server https://control-plane.example.com --recovery-token <token>
```

If the agent cannot find a `principal_id` locally, you can supply it:

```bash
xentz-agent recover --server https://control-plane.example.com --recovery-token <token> --principal-id <principal_id>
```

Notes:
- The recovery token is **one-time** and **short-lived** (server default: 15 minutes).
- Recovery rotates the principal API key; old keys stop working.
