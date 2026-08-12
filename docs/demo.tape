# Agent Session demo — one session, any coding agent.
# Reproduce: vhs docs/demo.tape

Output docs/demo.gif

Set Shell "bash"
Set Width 1200
Set Height 640
Set Padding 24
Set FontSize 16
Set Theme "Dracula"

# initial blank line
Sleep 400ms
Type "cd /var/folders/y9/741bchnj3ln14j_yvwmqp11w0000gn/T/opencode/demo"
Enter
Sleep 500ms

# header comment
Type "# One session. Any coding agent. — switch agents without losing context."
Enter
Sleep 300ms

Type "agent-session init --no-agents"
Enter
Sleep 1500ms

Type "agent-session start 'Add OAuth2 PKCE login'"
Enter
Sleep 900ms

Type "agent-session status"
Enter
Sleep 1200ms

Type "agent-session checkpoint --next-action 'Implement token rotation'"
Enter
Sleep 900ms

Type "agent-session handoff codex"
Enter
Sleep 1500ms

Type "agent-session resume --agent codex"
Enter
Sleep 1500ms

Type "agent-session context"
Enter
Sleep 900ms

# leave the output visible
Sleep 800ms
