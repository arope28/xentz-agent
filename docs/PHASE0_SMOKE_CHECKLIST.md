# Phase 0 – Baseline smoke checklist

Use this before Phase 1 (billing) or Phase 2 (alerts) work so you know enroll → backup → report → kill switch works end-to-end.

**Prerequisites**

- Built control plane from the **control plane** repository: README quick start (`XENTZ_MASTER_KEY`, `XENTZ_CONTROL_BASE`, `XENTZ_REPO_BASE`, listen address, DB path).
- Install token or invite code configured on the server.
- `restic` on the agent machine; repo backend reachable from test environment.

**Steps**

1. **Start control plane** with required env; confirm process listens (e.g. `127.0.0.1:9000`).
2. **Portal (optional but recommended):** Open `/ui/login`, sign in with bootstrap admin (`XENTZ_PORTAL_ADMIN_EMAIL` / `PASSWORD`), confirm `/ui/devices` loads.
3. **Enroll agent:**  
   `xentz-agent install --token <token> --server <control-base-url> --daily-at 02:00 --include <path>`  
   Confirm success output shows tenant ID, device ID, repository.
4. **Confirm server state:** Device row exists in portal (or DB); at least one config revision.
5. **Run backup:**  
   `xentz-agent backup`  
   Expect success exit; snapshot in restic; run visible on device detail / metrics.
6. **Kill switch:** In portal, revoke device; run `xentz-agent backup` again. Expect clear failure (device disabled / kill-switch), not a silent success.
7. **Unrevoke:** Confirm backup succeeds again after unrevoke.

**Exit:** All steps pass; any failure is tracked (bug or env). No production billing or alerts required for this phase.

**Automated helper (optional):** In the control plane repo, `scripts/smoke-phase0-curl.sh` posts `POST /v1/install` (set `CONTROL` and `INVITE`); then run the agent on a real host.
