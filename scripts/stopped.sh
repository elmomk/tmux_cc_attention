#!/usr/bin/env bash
# Called by Claude Code's Stop hook.
# Turns the tmux window label blue to indicate Claude has stopped.

[ -z "$TMUX" ] && exit 0
[ -z "$TMUX_PANE" ] && exit 0

# Close stdin immediately (no fork, no blocking)
exec 0</dev/null

target=$(tmux display-message -p -t "$TMUX_PANE" '#{session_name}:#{window_index}') || exit 0

# Read cached format strings (precomputed at plugin load)
fmt=$(tmux show-option -gqv @claude-fmt-stopped)
fmt_cur=$(tmux show-option -gqv @claude-fmt-stopped-cur)

tmux set-window-option -t "$target" window-status-format "$fmt" 2>/dev/null
tmux set-window-option -t "$target" window-status-current-format "$fmt_cur" 2>/dev/null

# Set stopped marker, clear active and attention
tmux set-window-option -t "$target" @claude-stopped 1 2>/dev/null
tmux set-window-option -t "$target" -u @claude-active 2>/dev/null
tmux set-window-option -t "$target" -u @claude-attention 2>/dev/null
