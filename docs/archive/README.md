# Archived Docs

This folder contains historical planning, design, and review documents that are no longer the active source of truth.

## Why these files are archived

These documents are kept for context and audit history, but they can drift from implementation over time.
Active operational guidance should come from current docs in `docs/` and from the codebase.

## Archived files

- `ARCHITECTURE.md` - older architecture snapshot
- `FLOW.md` - older flow diagrams and endpoint narrative
- `SECURITY_REVIEW.md` - historical security review snapshot
- `IMPROVEMENTS.md` - historical improvements tracker
- `RESTORE_UX_EXECUTION_PLAN.md` - restore UX execution planning snapshot
- `LOCAL_UI_RESTORE_MVP.md` - early restore UI/API MVP spec snapshot

## Active docs to use instead

- `../INSTALL.md` - install, modes, troubleshooting, local-ui, launcher usage
- `../RESTORE.md` - operator restore commands
- `../CLIENT_RECOVERY_GUIDE.md` - client-facing restore/recovery guidance
- `../NON_TECHNICAL_RESTORE_PLAYBOOK.md` - copy/paste restore flows for support
- `../REQUIREMENTS.md` - current v1 checklist/status

## Archival guideline

Move a doc to `docs/archive/` when:

- it is primarily planning/history and not runbook material
- it duplicates newer canonical docs
- it requires frequent updates to stay accurate but is no longer maintained

Keep docs in top-level `docs/` when they are current, user/operator-facing, and part of routine workflows.
