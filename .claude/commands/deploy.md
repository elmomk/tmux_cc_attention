Build the Go binary, deploy to the installed tmux plugin, and reload tmux.

Steps:
1. Run `go build -ldflags="-s -w" -o bin/claude-state ./cmd/claude-state/` — fail if build errors
2. Kill the running daemon: `kill $(cat /run/user/1000/claude-state/claude-state.pid 2>/dev/null) 2>/dev/null && sleep 0.3`
3. Copy binary to installed plugin: `cp bin/claude-state ~/.tmux/plugins/tmux_cc_attention/bin/claude-state`
4. Copy all changed scripts: `cp scripts/*.sh ~/.tmux/plugins/tmux_cc_attention/scripts/`
5. Copy claude-attention.tmux: `cp claude-attention.tmux ~/.tmux/plugins/tmux_cc_attention/claude-attention.tmux`
6. Reload tmux: `tmux source-file ~/.tmux.conf`
7. Wait 1 second, then verify daemon is running: `claude-state status | jq '.uptime'`
8. Report what was deployed and the daemon status
