# V1 Sell-Ready Automation Checklist

**Purpose**

Define the minimum automation required before selling to customers.  
Focus on money, access control, safety, and visibility.  
Avoid over-engineering v1.

**Status Legend**

- [✅] DONE (agent/control-plane) = Implemented in agent and/or control-plane
- [⚠️] PARTIAL = Partial support or ops dependency
- [ ] NOT IN SYSTEM = Not implemented in agent or control-plane

## Billing & Money (Must)

- [⚠️] Automated subscription billing (Stripe or equivalent) - Status: PARTIAL (webhook + enforcement scaffolding; ops: `docs/STRIPE_BILLING_SETUP.md` in control plane)
- [✅] Billing states exist: active / past_due / canceled - Status: DONE (control-plane schema + webhook handling)
- [✅] Billing status automatically controls service access - Status: DONE (control-plane enforcement)
- [✅] Grace period defined (e.g. 7 days) - Status: DONE (configurable)
- [✅] Past-due automatically disables service - Status: DONE (post-grace enforcement)
- [✅] Payment success automatically re-enables service - Status: DONE (webhook updates)
- [⚠️] No manual billing enforcement required - Status: PARTIAL (depends on Stripe Dashboard + metadata wiring per `docs/STRIPE_BILLING_SETUP.md`)

## Agent Enrollment

- [✅] Temporary enrollment token exists - Status: DONE (agent/control-plane)
- [✅] Token is single-use - Status: DONE (agent/control-plane)
- [✅] Token is time-limited - Status: DONE (agent/control-plane)
- [✅] Agent auto-registers with server - Status: DONE (agent/control-plane)
- [✅] Agent automatically receives long-lived API key - Status: DONE (agent/control-plane)
- [✅] Client never sees secrets or API keys - Status: DONE (agent/control-plane)

## Config Delivery

- [✅] Config served automatically from server - Status: DONE (agent/control-plane)
- [⚠️] Config tied to client_id - Status: PARTIAL (agent field; server enforcement)
- [⚠️] Config tied to device_id - Status: PARTIAL (agent field; server enforcement)
- [⚠️] Config tied to user_id - Status: PARTIAL (agent field; server enforcement)
- [✅] Agent auto-fetches config on interval - Status: DONE (default 5m when Windows service or `local-ui` runs; `XENTZ_CONFIG_REFRESH_INTERVAL` e.g. `5m`/`off`; launchd/systemd/cron paths still fetch on each scheduled run)
- [✅] Config validation before use - Status: DONE (agent/control-plane)
- [✅] Cached last-known-good config fallback - Status: DONE (agent/control-plane)
- [✅] One device config cannot affect another device - Status: DONE (agent/control-plane)

## Kill Switch (Non-Negotiable)

- [✅] Server-side disable (no client machine access) - Status: DONE (agent/control-plane)
- [✅] Disable overrides cached config - Status: DONE (agent/control-plane)
- [✅] API key revocation OR enabled=false flag exists - Status: DONE (agent/control-plane)
- [⚠️] Disable takes effect within minutes - Status: PARTIAL (kill-switch cached on refresh interval when Windows service / local-ui; otherwise next scheduled backup/retention)
- [✅] Disabling one client/device cannot affect others - Status: DONE (agent/control-plane)

## Monitoring & Alerts

### Core Metrics (Collect & Store)

- [✅] Agent heartbeat metric - Status: DONE (control-plane metrics)
- [✅] Backup success / failure metric - Status: DONE (agent/control-plane)
- [✅] Backup size metric - Status: DONE (agent/control-plane)
- [✅] Backup duration metric - Status: DONE (agent/control-plane)
- [✅] Last successful backup timestamp - Status: DONE (agent/control-plane)
- [✅] Backup target / destination reachable metric - Status: DONE (control-plane metrics)
- [✅] Authentication / authorization status metric - Status: DONE (control-plane metrics)
- [✅] Encryption / integrity verification metric - Status: DONE (control-plane metrics)
- [✅] Agent version / update status metric - Status: DONE (control-plane metrics)

### Alerts (Actionable Only)

**Delivery:** Prometheus + Alertmanager must be deployed and fire-drilled; see control plane `docs/OPS_RUNBOOK_ALERTS.md` (rules under `deploy/prometheus/alerts.yml`).

#### Critical Alerts (Must Have Before Selling)

- [⚠️] Alert: agent silent > 24 hours (workstations) - Status: PARTIAL (rules exist; delivery ops: control plane `docs/OPS_RUNBOOK_ALERTS.md`)
- [⚠️] Alert: agent silent > 12 hours (servers) - Status: PARTIAL (rules exist; delivery not wired)
- [⚠️] Alert: backup job failure - Status: PARTIAL (rules exist; delivery not wired)
- [⚠️] Alert: repeated backup failures (2+ consecutive failures) - Status: PARTIAL (rules exist; delivery not wired)
- [⚠️] Alert: no successful backup in last 24 hours - Status: PARTIAL (rules exist; delivery not wired)
- [⚠️] Alert: authentication or credential failure - Status: PARTIAL (rules exist; delivery not wired)
- [⚠️] Alert: backup storage usage > 70% - Status: PARTIAL (rules exist; delivery not wired)
- [⚠️] Alert: backup storage usage > 85% (critical) - Status: PARTIAL (rules exist; delivery not wired)
- [⚠️] Alert: alert delivery failure (email / webhook / Slack not delivered) - Status: PARTIAL (alert rules + Alertmanager config)

#### Security & Data Integrity Alerts

- [⚠️] Alert: abnormal backup size drop - Status: PARTIAL (rules exist; delivery not wired)
- [⚠️] Alert: abnormal backup size spike - Status: PARTIAL (rules exist; delivery not wired)
- [⚠️] Alert: encryption failure - Status: PARTIAL (rules exist; delivery not wired)
- [⚠️] Alert: backup integrity check failure - Status: PARTIAL (rules exist; delivery not wired)
- [⚠️] Alert: restore operation failure - Status: PARTIAL (rules exist; delivery not wired)

#### Operational / Configuration Alerts

- [⚠️] Alert: config validation failure - Status: PARTIAL (rules exist; delivery not wired)
- [⚠️] Alert: backup schedule modified - Status: PARTIAL (rules exist; delivery not wired)
- [⚠️] Alert: backup scope or exclusions changed - Status: PARTIAL (rules exist; delivery not wired)
- [⚠️] Alert: retention policy violation - Status: PARTIAL (rules exist; delivery not wired)
- [⚠️] Alert: agent version unsupported or update failed - Status: PARTIAL (rules exist; delivery not wired)
- [⚠️] Alert: license, subscription, or storage plan expiring - Status: PARTIAL (rules exist; delivery not wired)

### Reporting / Verification Checks (Non-Alert)

- [✅] Daily backup summary generated (success / failure / offline devices) - Status: DONE (control-plane)
- [✅] Weekly backup health review completed - Status: DONE (control-plane)
- [✅] Monthly backup report generated - Status: DONE (control-plane)
- [⚠️] Quarterly restore test performed and documented - Status: PARTIAL (control-plane support)
- [⚠️] Backup scope confirmed with client - Status: PARTIAL (control-plane support)
- [⚠️] Recovery procedure documented and client-safe - Status: PARTIAL (`docs/CLIENT_RECOVERY_GUIDE.md` here + control-plane `docs/RECOVERY.md` for tokens)

### Go-Live Confirmation

- [⚠️] All critical alerts tested and verified - Status: PARTIAL (go-live checklist support)
- [⚠️] Alert recipients confirmed (primary + backup contact) - Status: PARTIAL (go-live checklist support)
- [⚠️] One successful file-level restore completed - Status: PARTIAL (go-live checklist support)
- [⚠️] One successful folder or system restore completed - Status: PARTIAL (go-live checklist support)
- [⚠️] Backup & recovery explanation approved for client use - Status: PARTIAL (`docs/CLIENT_RECOVERY_GUIDE.md` + go-live checklist)

## Logging (Support Only)

- [✅] Local agent logs exist - Status: DONE (agent)
- [✅] Logs shipped to central server - Status: DONE (agent/control-plane)
- [✅] Logs tagged with client_id and device_id - Status: DONE (agent)
- [✅] Log rotation enabled - Status: DONE (agent)
- [✅] Logs supplement monitoring (not primary signal) - Status: DONE (policy documented)

## Automation Scope Agreed

**Automated for v1**

- [✅] Billing enforcement - Status: DONE (control-plane enforcement)
- [✅] Agent enrollment - Status: DONE (agent/control-plane)
- [✅] Config delivery - Status: DONE (agent/control-plane)
- [✅] Kill switch - Status: DONE (agent/control-plane)
- [⚠️] Monitoring and alerts - Status: PARTIAL (Prometheus rules + Alertmanager; delivery verified per ops runbook in control plane)

**Manual for v1**

- [⚠️] Client onboarding steps - Status: PARTIAL (SOP: `docs/CLIENT_ONBOARDING_SOP.md`; not in-product wizard)
- [ ] Pricing changes - Status: NOT IN SYSTEM
- [⚠️] Restores - Status: PARTIAL (agent CLI `restore`; guided: `docs/CLIENT_RECOVERY_GUIDE.md`; not push-button portal)
- [⚠️] Client communication - Status: PARTIAL (SOP templates in `docs/CLIENT_ONBOARDING_SOP.md`)

## Not Required Before Selling

- [✅] Client portal - Status: DONE (control-plane)
- [⚠️] Self-service UI - Status: PARTIAL (portal exists; limited self-service)
- [ ] Usage-based billing - Status: NOT IN SYSTEM
- [ ] Per-GB pricing - Status: NOT IN SYSTEM
- [ ] Automated restores - Status: NOT IN SYSTEM
- [ ] Fancy dashboards - Status: NOT IN SYSTEM
- [ ] Multi-region infrastructure - Status: NOT IN SYSTEM

## Final Go / No-Go Check

- [⚠️] Payment is automatic - Status: PARTIAL (webhook + enforcement scaffolding)
- [⚠️] Non-payment disables service automatically - Status: PARTIAL (billing enforcement + grace)
- [⚠️] One client cannot affect another - Status: PARTIAL (agent isolation; server enforcement assumed)
- [⚠️] Backup failures are detected automatically - Status: PARTIAL (alerts + Alertmanager setup required)
- [⚠️] Restores are possible when requested - Status: PARTIAL (CLI `restore` + `docs/CLIENT_RECOVERY_GUIDE.md`; operator drills via go-live checklist API)
