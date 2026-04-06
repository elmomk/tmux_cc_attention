Run a full integration test of the claude-state daemon.

Steps:
1. Build: `go build -ldflags="-s -w" -o bin/claude-state ./cmd/claude-state/`
2. Kill any running daemon
3. Delete the state file to start fresh
4. Start daemon: `bin/claude-state serve &` and wait for socket
5. Test each state transition using TMUX=$TMUX TMUX_PANE=$TMUX_PANE:
   - `bin/claude-state active` → verify `@claude-active=1` on the window
   - `bin/claude-state attention` → verify `@claude-attention=1`
   - `bin/claude-state stopped` → verify `@claude-stopped=1`
   - `bin/claude-state clear` → verify markers cleared
6. Test status API: `bin/claude-state status` → verify JSON response has correct state
7. Test persistence: kill daemon, restart, verify state was restored from file
8. Test input validation: send malicious JSON via python socket, verify `{"ok":false}`
9. Test discovery: kill daemon, delete state file, restart, verify it discovers Claude windows
10. Kill test daemon
11. Report pass/fail for each test
