#!/usr/bin/env bash
# Auto-clear: run by session-window-changed hook.
# Clears attention/stopped markers when the user focuses a window,
# since they're now looking at it.

target=$(tmux display-message -p '#{session_name}:#{window_index}') || exit 0

# Read current markers in a single call
markers=$(tmux display-message -t "$target" -p '#{@claude-attention}|#{@claude-stopped}')
att="${markers%%|*}"
stop="${markers##*|}"

# Nothing to clear
[ "$att" != "1" ] && [ "$stop" != "1" ] && exit 0

# If attention or stopped, clear the visual state (keep active — user may glance mid-work)
if [ "$att" = "1" ]; then
    tmux set-window-option -t "$target" -u window-status-format \; \
         set-window-option -t "$target" -u window-status-current-format \; \
         set-window-option -t "$target" -u @claude-attention 2>/dev/null
elif [ "$stop" = "1" ]; then
    tmux set-window-option -t "$target" -u window-status-format \; \
         set-window-option -t "$target" -u window-status-current-format \; \
         set-window-option -t "$target" -u @claude-stopped 2>/dev/null
fi

# Push-update cross-session counts
"$(dirname "$0")/refresh-counts.sh" &
disown
