#!/usr/bin/env bash
# Manual clear: removes all Claude state indicators from a window.
# Not called automatically — state persists until it changes.

target=$(tmux display-message -p '#{session_name}:#{window_index}') || exit 0

# Unset per-window format overrides and remove all markers (single IPC call)
tmux set-window-option -t "$target" -u window-status-format \; \
     set-window-option -t "$target" -u window-status-current-format \; \
     set-window-option -t "$target" -u @claude-attention \; \
     set-window-option -t "$target" -u @claude-active \; \
     set-window-option -t "$target" -u @claude-stopped 2>/dev/null
