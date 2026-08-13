# Renders docs/demo.gif. Run from the repository root:
#
#   vhs docs/demo.tape
#
# docs/demo-setup.sh builds agent-session from this checkout and points HOME at
# a throwaway directory, so the recording is reproducible and never touches the
# real agent configuration.

Output "docs/demo.gif"

Require go
Require git

Set Shell "bash"
Set FontFamily "JetBrainsMono Nerd Font Mono"
Set FontSize 18
Set LineHeight 1.2
Set Width 1360
Set Height 820
Set Padding 24
Set Margin 60
Set MarginFill "#07141A"
Set BorderRadius 12
Set WindowBar Colorful
Set WindowBarSize 40
Set TypingSpeed 32ms
Set Framerate 24
Set CursorBlink false
Set Theme { "background": "#0B0F14", "foreground": "#CDD6E4", "cursor": "#2DD4BF", "selection": "#1F2A33", "black": "#161B22", "red": "#F87171", "green": "#4ADE80", "yellow": "#FBBF24", "blue": "#60A5FA", "magenta": "#C084FC", "cyan": "#2DD4BF", "white": "#CDD6E4", "brightBlack": "#5A6B7B", "brightRed": "#FCA5A5", "brightGreen": "#86EFAC", "brightYellow": "#FCD34D", "brightBlue": "#93C5FD", "brightMagenta": "#D8B4FE", "brightCyan": "#5EEAD4", "brightWhite": "#F1F5F9" }

Hide
Type "source docs/demo-setup.sh" Enter
Wait+Screen@120s /DEMO-READY/
Type "clear" Enter
Sleep 1s
Show

Sleep 1s
Type "# one command wires every coding agent on this machine" Sleep 400ms Enter
Sleep 600ms
Type "agent-session init" Sleep 400ms Enter
Sleep 3s

Type "# the session belongs to the project, not to one agent" Sleep 400ms Enter
Sleep 600ms
Type "agent-session start 'Add OAuth2 PKCE login'" Sleep 400ms Enter
Sleep 3s

Type "# your agent records tasks, decisions and next steps into it" Sleep 400ms Enter
Sleep 600ms
Type "agent-session status" Sleep 400ms Enter
Sleep 4s

Type "# out of context? hand the whole session to another agent" Sleep 400ms Enter
Sleep 600ms
Type "agent-session handoff codex" Sleep 400ms Enter
Sleep 5s
Type "# nothing to re-explain."
Sleep 3s
