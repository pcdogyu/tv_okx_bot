# Agent Delivery Rules

This file is the standing prompt for future AI agents working in this repo.

## User Preference

The user expects completed modifications to be committed, pushed to the remote branch, deployed to the configured production server, and verified automatically. Treat this as the default habit for this project so the user does not need to repeat it in future conversations.

## Default Delivery Behavior

After any code, config, script, or documentation change is completed, automatically publish it and trigger the production upgrade. Do not ask the user to repeat this instruction.

Skip the publish/upgrade step only when the user explicitly says one of these:

- "do not push"
- "do not deploy"
- "do not upgrade"
- "local only"
- "just inspect/review/explain"

## Required Checklist

1. Inspect the worktree:

   ```powershell
   git status --short
   git diff --stat
   ```

2. Protect unrelated user changes. Commit only files that belong to the current task. Never revert unrelated changes unless the user explicitly asks.

3. Run verification before publishing. For this Go service, default to:

   ```powershell
   go test ./...
   ```

4. Commit the task-related changes:

   ```powershell
   git add <task-related-files>
   git commit -m "<short task summary>"
   ```

5. Push the current branch to origin:

   ```powershell
   $branch = git branch --show-current
   git push origin $branch
   ```

6. Trigger the server upgrade through the production endpoint:

   ```powershell
   curl.exe -u "$env:TVBOT_ADMIN_USER:$env:TVBOT_ADMIN_PASSWORD" -X POST https://tvbot.lmitis.com/upgrade
   ```

   If an admin token is available instead of Basic Auth:

   ```powershell
   curl.exe -H "X-Admin-Token: $env:ADMIN_TOKEN" -X POST https://tvbot.lmitis.com/upgrade
   ```

7. Check upgrade status before the final response:

   ```powershell
   curl.exe -u "$env:TVBOT_ADMIN_USER:$env:TVBOT_ADMIN_PASSWORD" https://tvbot.lmitis.com/upgrade
   ```

   Or with token:

   ```powershell
   curl.exe -H "X-Admin-Token: $env:ADMIN_TOKEN" https://tvbot.lmitis.com/upgrade
   ```

8. Final response must include:

   - verification result
   - commit hash
   - pushed branch
   - upgrade trigger/status result
   - any failure that prevented publish or upgrade

## Reusable Prompt

Use this prompt at the start of a future conversation when needed:

```text
Follow AGENTS.md for this repo. After development is complete, run verification, commit only the current task changes, push the current branch to origin, POST https://tvbot.lmitis.com/upgrade, then GET /upgrade to confirm status. Do not ask again unless I explicitly say local-only, do not push, or do not upgrade.
```
