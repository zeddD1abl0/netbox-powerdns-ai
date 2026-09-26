#!/usr/bin/env bash
# PreToolUse guard (ADR-0010): Claude commits only on milestone branches.
# Denies a Bash call that runs a git subcommand that creates commits (commit,
# merge, cherry-pick, revert, am, pull, rebase) while the project is on main,
# or that switches to main and runs one in the same call. It matches the
# command's text, so it's a safety net against mistakes, not a sandbox.
input=$(cat)
# Take the command from the hook input. Without jq, fall back to the whole
# input: that can only produce extra denials, never miss a commit.
if command -v jq >/dev/null 2>&1; then
	cmd=$(jq -r '.tool_input.command // empty' <<<"$input")
else
	cmd=$input
fi

# `git`, perhaps by path (/usr/bin/git), then any global options, then the
# subcommand as its own word. Options that take a separate argument (-c, -C,
# --git-dir, …) may have it quoted; other options are single words.
git_start='(^|[^A-Za-z0-9_./-])(/?([A-Za-z0-9_.~-]+/)+)?git'
arg="('[^']*'|\"[^\"]*\"|[^[:space:]]+)"
opts="([[:space:]]+(-[cC]|--(git-dir|work-tree|namespace|super-prefix|config-env|exec-path))[[:space:]]+${arg}|[[:space:]]+--?[A-Za-z][^[:space:]]*)*"
word_end='([^A-Za-z0-9_-]|$)'
git_cmd() { grep -Eq "${git_start}${opts}[[:space:]]+$1${word_end}" <<<"$cmd"; }

git_cmd '(commit|merge|cherry-pick|revert|am|pull|rebase)' || exit 0
branch=$(git -C "${CLAUDE_PROJECT_DIR:-.}" branch --show-current 2>/dev/null)
if [ "$branch" != main ] && ! git_cmd '(switch|checkout)([[:space:]]+-[^[:space:]]+)*[[:space:]]+main'; then
	exit 0
fi
cat <<'JSON'
{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"Claude commits, merges, cherry-picks and rebases only on milestone branches, never on main (ADR-0010). Switch to the milestone branch (mNN-short-title) first."}}
JSON
