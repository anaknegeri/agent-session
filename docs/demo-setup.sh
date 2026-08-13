#!/usr/bin/env bash
# Fixture for docs/demo.tape. Source it (never execute it) from the repository
# root: it replaces HOME and cd's into a throwaway git project so the recording
# shows a clean, anonymous shell and can never touch the real agent configs.
#
#   source docs/demo-setup.sh
#
# It builds agent-session from the working tree, so the GIF always shows the
# code in this checkout rather than whatever is installed on PATH.

demo_root="$(mktemp -d)"
demo_repo="$PWD"

export HOME="$demo_root"
export CODEX_HOME="$demo_root/.codex"
export GIT_AUTHOR_NAME="demo"
export GIT_AUTHOR_EMAIL="demo@example.com"
export GIT_COMMITTER_NAME="$GIT_AUTHOR_NAME"
export GIT_COMMITTER_EMAIL="$GIT_AUTHOR_EMAIL"
export GIT_CONFIG_GLOBAL="$demo_root/.gitconfig"
mkdir -p "$CODEX_HOME"

go build -o "$demo_root/bin/agent-session" "$demo_repo/cmd/agent-session" || return 1
export PATH="$demo_root/bin:$PATH"

mkdir -p "$demo_root/checkout-api/internal/auth"
cd "$demo_root/checkout-api" || return 1
git init -q -b main

cat > internal/auth/pkce.go <<'GO'
package auth

// Verifier is the high-entropy secret a public client keeps to itself.
type Verifier string

func NewVerifier() (Verifier, error) { return "", nil }
GO

cat > internal/auth/token.go <<'GO'
package auth

type Token struct {
	Access  string
	Refresh string
}
GO

echo ".agent/" > .gitignore
git add -A
git commit -qm "feat(auth): PKCE skeleton"

# One uncommitted edit, so the session has a real changed file to report rather
# than only its own bookkeeping.
printf '\nfunc (v Verifier) Challenge() string { return "" }\n' >> internal/auth/pkce.go

# demo_mcp speaks JSON-RPC to the MCP server the way an agent does: each argument
# is one tools/call, prefixed with the initialize handshake.
demo_mcp() {
	{
		printf '%s\n' '{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"demo","version":"1"}}}'
		printf '%s\n' '{"jsonrpc":"2.0","method":"notifications/initialized"}'
		printf '%s\n' "$@"
	} | agent-session mcp 2>/dev/null
}

# demo_seed writes the tasks, decision and next action an agent would normally
# record through the MCP tools, so status and handoff show a session with real
# content instead of an empty shell.
demo_seed() {
	local done_id
	done_id=$(demo_mcp \
		'{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"task.create","arguments":{"title":"Generate and store the code verifier"}}}' \
		| grep -o 'task_[0-9a-f]*' | head -1)

	demo_mcp \
		"{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"task.update\",\"arguments\":{\"task_id\":\"$done_id\",\"status\":\"completed\"}}}" \
		'{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"task.create","arguments":{"title":"Wire the PKCE challenge into /authorize"}}}' \
		'{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"decision.create","arguments":{"decision":"PKCE over the implicit flow","reason":"a public client cannot keep a secret"}}}' \
		> /dev/null
}

# demo_watch waits for the recorded `agent-session start` to create the session,
# then fills it in. It runs in the background from here rather than from the tape
# because a Hide'd command still lands in the scrollback and would show up on
# screen the moment recording resumes.
demo_watch() {
	local i=0
	while [ "$i" -lt 120 ]; do
		if agent-session status 2>/dev/null | grep -q "PKCE"; then
			demo_seed
			agent-session checkpoint --next-action "Verify the challenge against RFC 7636 vectors" > /dev/null 2>&1
			agent-session resume --agent claude > /dev/null 2>&1
			return 0
		fi
		sleep 0.25
		i=$((i + 1))
	done
	return 1
}

( demo_watch & ) > /dev/null 2>&1

export PS1='\[\e[38;2;94;234;212m\]checkout-api\[\e[0m\] \[\e[38;2;125;211;252m\]❯\[\e[0m\] '
clear
echo "DEMO-READY"
