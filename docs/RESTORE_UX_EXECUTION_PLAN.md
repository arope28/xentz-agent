# Restore UX execution plan (target: easy + high confidence)

Goal: move practical user-mode restore ratings from:

- Backup setup: medium
- Checking if protected: medium-easy
- Self-serve restore one file/folder: medium-hard
- Confidence without support: low-medium

to:

- Backup setup: easy
- Checking if protected: easy
- Self-serve restore one file/folder: easy
- Confidence without support: high

---

## Success metrics

Track per release cohort:

1. **First-try restore success rate** (user mode, no support) >= 90%
2. **Median time to restore one file** <= 3 minutes
3. **Support tickets for restore confusion** reduced by >= 60%
4. **Mode mismatch failures** reduced to near-zero via guardrails

Instrumentation inputs:
- CLI command outcomes (`restore`, new guided flows)
- Error category counters (permission, auth, not-found, mode mismatch)
- Local UI restore completion events

---

## Phase 1 (fast, low risk): make current flow simpler now

### Deliverables

1. **Non-technical playbook** (done): `NON_TECHNICAL_RESTORE_PLAYBOOK.md`
2. **Quick-path docs in INSTALL/RECOVERY**
   - "Restore latest one file"
   - "Restore folder to Desktop"
   - "Restore full snapshot safely to alternate path"
3. **Better error copy** in restore command output
   - Suggest exact next command when common errors occur (`find`, `doctor`, FDA checklist)

### Acceptance

- A support agent can run restores with copy/paste commands only.
- Users can complete common restores with one provided command block.

---

## Phase 2: guided restore command (highest UX impact)

Add interactive subcommand:

```bash
xentz-agent restore guided
```

### Prompt flow

1. Choose restore type:
   - one file
   - one folder
   - full snapshot
2. Choose snapshot:
   - latest
   - pick by date/time from list
3. Enter/select source path (with validation)
4. Choose destination:
   - safe default (`~/Desktop/xentz-restore-*`)
   - custom path
5. Confirm summary and execute
6. Show exact output path + open-folder hint

### Safety rules

- Never overwrite by default
- Require explicit confirmation for original-location restore
- Validate dangerous targets (root, system dirs)

### Acceptance

- Non-technical user can restore one file from latest without knowing snapshot IDs.

---

## Phase 3: local UI restore workflow

Extend `local-ui` with restore pages:

1. Snapshot list with human timestamps
2. File/folder find input
3. Restore wizard (target + confirmation)
4. Progress + final restored path

### UX constraints

- Keep localhost-only security model
- Reuse same backend restore execution path as CLI
- Display copyable fallback CLI command for advanced support

### Acceptance

- User can complete restore from browser without terminal knowledge.

---

## Phase 4: confidence and supportability

### Diagnostics

- Extend `doctor` with restore-readiness check:
  - mode alignment
  - server auth
  - restic presence
  - writable restore destination test
  - macOS FDA hints when likely needed

### Documentation

- Add one-page "Restore in 3 clicks/commands" quick reference
- Add screenshots (local UI)
- Add support decision tree by error category

### Acceptance

- Support can triage restore failures in under 5 minutes.

---

## Implementation order and effort

1. **Phase 1**: 1 sprint, low engineering risk
2. **Phase 2**: 1 sprint, medium risk (interactive UX + validation)
3. **Phase 3**: 1-2 sprints, medium/high risk (UI + workflow)
4. **Phase 4**: parallel hardening/documentation pass

---

## Risks and mitigations

1. **Mode/context confusion persists**
   - Mitigation: keep mode mismatch guardrails in all user-facing commands
2. **macOS permissions create false negatives**
   - Mitigation: FDA checklist integration + targeted hints in errors/doctor
3. **Path confusion in snapshots**
   - Mitigation: guided path finder + defaults + explicit destination echo

---

## Immediate next backlog items

1. Implement `restore guided` CLI command (Phase 2)
2. Add quick-path section to `docs/RESTORE.md` and `docs/CLIENT_RECOVERY_GUIDE.md`
3. Add restore-readiness checks to `doctor`
4. Add local UI restore MVP spec and endpoint contracts
