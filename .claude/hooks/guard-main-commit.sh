#!/usr/bin/env bash
# PreToolUse guard (ADR-0010): Claude commits only on milestone branches.
# Denies a Bash call that runs `git … commit` while the project is on main, or
# that switches to main and commits in the same call.
input=$(cat)
# Take the command from the hook input. Without jq, fall back to the whole
# input: that can only produce extra denials, never miss a commit.
if command -v jq >/dev/null 2>&1; then
	cmd=$(jq -r '.tool_input.command // empty' <<<"$input")
else
	cmd=$input
fi

# `git`, any global options (-c k=v, -C dir, --no-pager, --git-dir=…), then
# the subcommand as its own word.
opts='([[:space:]]+-[cC][[:space:]]+[^[:space:]]+|[[:space:]]+--?[A-Za-z][^[:space:]]*)*'
word_end='([^A-Za-z0-9_-]|$)'
git_cmd() { grep -Eq "(^|[^A-Za-z0-9_./-])git${opts}[[:space:]]+$1${word_end}" <<<"$cmd"; }

git_cmd commit || exit 0
branch=$(git -C "${CLAUDE_PROJECT_DIR:-.}" branch --show-current 2>/dev/null)
if [ "$branch" != main ] && ! git_cmd '(switch|checkout)([[:space:]]+-[^[:space:]]+)*[[:space:]]+main'; then
	exit 0
fi
cat <<'JSON'
{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"Claude commits only on milestone branches, never on main (ADR-0010). Switch to the milestone branch (mNN-short-title) first."}}
JSON
