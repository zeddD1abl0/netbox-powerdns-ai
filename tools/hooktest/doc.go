// Package hooktest tests the Claude Code hooks configured in .claude/: the
// commit guard (.claude/hooks/guard-main-commit.sh) and the goimports edit
// hook in .claude/settings.json. It has no code of its own, only tests, which
// pipe hook input into the real hooks the way Claude Code does.
//
// `make test` runs them with the pinned jq and golangci-lint, passed in
// HOOKTEST_JQ and HOOKTEST_GOLANGCI_LINT. Run directly with `go test`, the
// tests look for jq and golangci-lint on PATH, and skip what they can't run.
package hooktest
