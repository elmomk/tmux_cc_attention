#!/usr/bin/env bash
# Called by Claude Code's Notification hook (idle_prompt matcher).
# Turns the tmux window label amber to indicate Claude is idle/waiting.

[ -z "$TMUX" ] && exit 0
[ -z "$TMUX_PANE" ] && exit 0

# Drain stdin in background (polite to Claude Code, non-blocking for us)
cat > /dev/null &

target=$(tmux display-message -p -t "$TMUX_PANE" '#{session_name}:#{window_index}') || exit 0

# Read cached format strings (precomputed at plugin load)
fmt=$(tmux show-option -gqv @claude-fmt-idle)
fmt_cur=$(tmux show-option -gqv @claude-fmt-idle-cur)

tmux set-window-option -t "$target" window-status-format "$fmt" 2>/dev/null
tmux set-window-option -t "$target" window-status-current-format "$fmt_cur" 2>/dev/null

# Set idle marker, clear others
tmux set-window-option -t "$target" @claude-idle 1 2>/dev/null
tmux set-window-option -t "$target" -u @claude-active 2>/dev/null
tmux set-window-option -t "$target" -u @claude-attention 2>/dev/null
tmux set-window-option -t "$target" -u @claude-stopped 2>/dev/null
