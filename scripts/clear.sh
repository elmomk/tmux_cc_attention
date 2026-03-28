#!/usr/bin/env bash
# Manual clear: removes all Claude state indicators from a window.
# Not called automatically — state persists until it changes.

target=$(tmux display-message -p '#{session_name}:#{window_index}') || exit 0

# Unset per-window format overrides (reverts to global)
tmux set-window-option -t "$target" -u window-status-format 2>/dev/null
tmux set-window-option -t "$target" -u window-status-current-format 2>/dev/null
# Remove all markers
tmux set-window-option -t "$target" -u @claude-attention 2>/dev/null
tmux set-window-option -t "$target" -u @claude-active 2>/dev/null
tmux set-window-option -t "$target" -u @claude-stopped 2>/dev/null
