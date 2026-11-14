# Records demo/demo.gif. Run from the repository root:
#
#   make demo
#
# The sessions run demo/stub/claude, a stub that imitates Claude Code output,
# so recording needs neither an API key nor a real project.

Output demo/demo.gif

Require tmux

Set Shell bash
Set FontSize 14
Set Width 1400
Set Height 700
Set Padding 16
Set TypingSpeed 80ms

Hide
Type "source demo/setup.sh" Enter
Sleep 3s
Show

# Dashboard: one active, one idle and one waiting session
Type "claude-dashboard" Sleep 500ms Enter
Sleep 4s
Screenshot demo/shots/1-dashboard.png

# Logs of the session that is waiting for an answer
Down Sleep 600ms
Down Sleep 1s
Type "l" Sleep 3s
Screenshot demo/shots/2-logs.png
Escape Sleep 1s

# Detail view
Type "d" Sleep 3s
Screenshot demo/shots/3-detail.png
Escape Sleep 1s

# Filter
Type "/" Sleep 500ms
Type "api" Sleep 1s
Enter Sleep 2s
Screenshot demo/shots/4-filter.png
Type "/" Sleep 300ms
Escape Sleep 1s

# New session
Type "n" Sleep 1s
Type "docs" Sleep 1s
Screenshot demo/shots/5-create.png
Enter Sleep 3s
Screenshot demo/shots/6-created.png

# Attach to the active session, then detach back to the dashboard
Up Sleep 300ms
Up Sleep 300ms
Up Sleep 300ms
Up Sleep 600ms
Enter Sleep 4s
Screenshot demo/shots/7-attached.png
Ctrl+B Sleep 300ms
Type "d" Sleep 3s
Screenshot demo/shots/8-detached.png

# Help
Type "?" Sleep 3s
Screenshot demo/shots/9-help.png
Escape Sleep 1s

Type "q" Sleep 1s
