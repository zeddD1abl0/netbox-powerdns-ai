package hooktest

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A guardCase is one Bash command piped into the commit guard while the
// project is on branch.
type guardCase struct {
	name    string
	branch  string
	command string
	deny    bool
}

var guardCases = []guardCase{
	// On main, every git subcommand that creates commits is denied, however
	// git is spelled and whatever global options come first.
	{"commit", "main", "git commit -m x", true},
	{"amend", "main", "git commit --amend --no-edit", true},
	{"git by path", "main", "/usr/bin/git commit -m x", true},
	{"after cd", "main", "cd /tmp && git commit -m x", true},
	{"after make", "main", "make check && git commit -m x", true},
	{"in a subshell", "main", "(git commit -m x)", true},
	{"on a second line", "main", "git status\ngit commit -m x", true},
	{"after a tab", "main", "git\tcommit -m x", true},
	{"-C dir", "main", "git -C . commit -m x", true},
	{"-C quoted dir", "main", `git -C "a b" commit -m x`, true},
	{"-c quoted value", "main", "git -c 'user.name=A B' commit -m x", true},
	{"--git-dir with argument", "main", "git --git-dir .git commit -m x", true},
	{"--git-dir= and --work-tree=", "main", "git --git-dir=.git --work-tree=. commit -m x", true},
	{"--no-pager", "main", "git --no-pager commit -m x", true},
	{"escaped quotes in message", "main", `git commit -m "fix: \"quoted\""`, true},
	{"merge", "main", "git merge feature", true},
	{"cherry-pick", "main", "git cherry-pick abc123", true},
	{"revert", "main", "git revert HEAD", true},
	{"am", "main", "git am < fix.patch", true},
	{"pull", "main", "git pull", true},
	{"rebase", "main", "git rebase feature", true},

	// On main, commands that don't create commits are allowed.
	{"status", "main", "git status", false},
	{"diff", "main", "git diff --stat", false},
	{"log grep merge", "main", "git log --grep merge", false},
	{"mergetool", "main", "git mergetool", false},
	{"commit-tree", "main", "git commit-tree HEAD^{tree} -m x", false},
	{"echo", "main", "echo git committed", false},
	{"legit", "main", "legit commit", false},
	{"no git", "main", "ls commit", false},
	// Outside the guard's scope, by design: these move main without a
	// subcommand that creates commits. The guard is a safety net against
	// mistakes, not a sandbox.
	{"fetch into main", "main", "git fetch . HEAD:main", false},
	{"branch -f main", "main", "git branch -f main HEAD", false},
	{"update-ref main", "main", "git update-ref refs/heads/main HEAD", false},

	// On a milestone branch, commits are allowed, unless the same command
	// switches to main first.
	{"commit on a branch", "m01-x", "git commit -m x", false},
	{"merge main into a branch", "m01-x", "git merge main", false},
	{"status on a branch", "m01-x", "git status", false},
	{"switch to main, then commit", "m01-x", "git switch main && git commit -m x", true},
	{"switch -q to main, then commit", "m01-x", "git switch -q main && git commit -m x", true},
	{"checkout main, then merge", "m01-x", "git checkout main; git merge m01-x", true},
	{"switch to main-fix, then commit", "m01-x", "git switch main-fix && git commit -m x", false},
	// Outside the guard's scope: it checks the project's branch, not the
	// branch of a worktree that a command names.
	{"commit in a worktree on main", "m01-x", "git worktree add ../wt main && git -C ../wt commit -m x", false},
}

// TestGuard pipes each case into the PreToolUse hook that
// .claude/settings.json registers for Bash, in a scratch repository on the
// case's branch. It runs every case twice: with jq, and through the guard's
// fallback for hosts without jq, which must decide the same way.
func TestGuard(t *testing.T) {
	command := hookCommand(t, "PreToolUse", "Bash")
	guard := filepath.Join(root(t), ".claude", "hooks", "guard-main-commit.sh")
	// Each scratch repository links to the real guard where the hook command
	// expects it, relative to CLAUDE_PROJECT_DIR. Running it through the link
	// also checks that the guard is executable.
	repos := map[string]string{}
	for _, branch := range []string{"main", "m01-x"} {
		dir := t.TempDir()
		git := exec.CommandContext(t.Context(), "git", "init", "-q", "-b", branch, dir)
		git.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		if out, err := git.CombinedOutput(); err != nil {
			t.Fatalf("git init: %v\n%s", err, out)
		}
		hooks := filepath.Join(dir, ".claude", "hooks")
		if err := os.MkdirAll(hooks, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(guard, filepath.Join(hooks, "guard-main-commit.sh")); err != nil {
			t.Fatal(err)
		}
		repos[branch] = dir
	}

	// What the guard runs, and nothing else, so jq is present only where a
	// mode puts it.
	cmds := map[string]string{}
	for _, name := range []string{"bash", "cat", "grep", "git"} {
		cmds[name] = hostTool(t, name)
	}
	home := t.TempDir()

	for _, withJQ := range []bool{true, false} {
		mode := "without jq"
		if withJQ {
			mode = "with jq"
		}
		t.Run(mode, func(t *testing.T) {
			bin := map[string]string{}
			for k, v := range cmds {
				bin[k] = v
			}
			if withJQ {
				bin["jq"] = pinnedTool(t, "HOOKTEST_JQ", "jq")
			}
			path := binDir(t, bin)
			for _, c := range guardCases {
				t.Run(c.name, func(t *testing.T) {
					input := map[string]any{
						"session_id":      "hooktest",
						"hook_event_name": "PreToolUse",
						"tool_name":       "Bash",
						"tool_input":      map[string]string{"command": c.command, "description": "Run a hook test"},
					}
					env := []string{
						"PATH=" + path,
						"HOME=" + home,
						"CLAUDE_PROJECT_DIR=" + repos[c.branch],
						"GIT_CONFIG_GLOBAL=/dev/null",
						"GIT_CONFIG_NOSYSTEM=1",
					}
					out := runHook(t, command, input, env)
					if denied := decision(t, out) == "deny"; denied != c.deny {
						t.Errorf("on %s, %q: denied = %v, want %v (output %q)", c.branch, c.command, denied, c.deny, out)
					}
				})
			}
		})
	}
}

// decision returns the permission decision in a PreToolUse hook's output,
// or "" when the hook printed nothing, which lets the call go ahead.
func decision(t *testing.T, out string) string {
	t.Helper()
	if out == "" {
		return ""
	}
	var v struct {
		HookSpecificOutput struct {
			HookEventName            string `json:"hookEventName"`
			PermissionDecision       string `json:"permissionDecision"`
			PermissionDecisionReason string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("hook output isn't JSON: %v: %q", err, out)
	}
	h := v.HookSpecificOutput
	if h.HookEventName != "PreToolUse" || h.PermissionDecisionReason == "" {
		t.Errorf("hook output is incomplete: %q", out)
	}
	return h.PermissionDecision
}
