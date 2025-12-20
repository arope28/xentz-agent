xentz-agent - Backup Agent

Quick Start:
1. Install restic: brew install restic
2. Run: ./xentz-agent install --repo <your-repo> --password <password> --include <paths>

Example:
  ./xentz-agent install --repo rest:https://your-repo.com/backup --password "your-password" --include "/Users/yourname/Documents"

Commands:
  ./xentz-agent install --repo <url> --password <pwd> --include <paths>
  ./xentz-agent backup
  ./xentz-agent status
