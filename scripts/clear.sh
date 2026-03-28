#!/usr/bin/env bash
# Called by tmux session-window-changed / client-session-changed hooks.
# Clears attention, active, or stopped highlight when the user switches to the window.

target=$(tmux display-message -p '#{session_name}:#{window_index}') || exit 0

attention=$(tmux show-window-option -t "$target" -v @claude-attention 2>/dev/null)
active=$(tmux show-window-option -t "$target" -v @claude-active 2>/dev/null)
stopped=$(tmux show-window-option -t "$target" -v @claude-stopped 2>/dev/null)

# Nothing to clear
[ "$attention" != "1" ] && [ "$active" != "1" ] && [ "$stopped" != "1" ] && exit 0

# Unset per-window format override (reverts to global)
tmux set-window-option -t "$target" -u window-status-format
# Remove all markers
tmux set-window-option -t "$target" -u @claude-attention 2>/dev/null
tmux set-window-option -t "$target" -u @claude-active 2>/dev/null
tmux set-window-option -t "$target" -u @claude-stopped 2>/dev/null
