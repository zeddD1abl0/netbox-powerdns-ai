#!/usr/bin/env bash
# PreToolUse guard (ADR-0010): Claude commits only on milestone branches.
# Reads the hook JSON on stdin and denies any Bash call that runs
# `git commit` while the project's current branch is main.
cmd=$(jq -r '.tool_input.command // empty')
printf '%s' "$cmd" | grep -Eq '(^|[;&|(]|[[:space:]])git([[:space:]]+-C[[:space:]]+[^[:space:]]+)?[[:space:]]+commit([[:space:]]|$)' || exit 0
branch=$(git -C "${CLAUDE_PROJECT_DIR:-.}" branch --show-current 2>/dev/null)
[ "$branch" = main ] || exit 0
cat <<'JSON'
{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"Claude commits only on milestone branches, never on main (ADR-0010). Switch to the milestone branch (mNN-short-title) first."}}
JSON
