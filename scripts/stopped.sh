#!/usr/bin/env bash
# Called by Claude Code's Stop hook.
# Turns the tmux window label blue to indicate Claude has stopped.

[ -z "$TMUX" ] && exit 0
[ -z "$TMUX_PANE" ] && exit 0

CURRENT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$CURRENT_DIR/helpers.sh"

# Consume stdin (Claude Code sends JSON payload)
cat > /dev/null

target=$(pane_to_window "$TMUX_PANE") || exit 0

# Always clear active (Claude has stopped working regardless)
tmux set-window-option -t "$target" -u @claude-active 2>/dev/null

# If attention is set: clear the marker so the next PreToolUse (when the user
# responds) can transition to active/green — but keep the red window format.
if [ "$(tmux show-window-option -t "$target" -v @claude-attention 2>/dev/null)" = "1" ]; then
    tmux set-window-option -t "$target" -u @claude-attention 2>/dev/null
    exit 0
fi

color=$(get_stopped_color | tr -cd 'a-zA-Z0-9#')

tmux set-window-option -t "$target" window-status-format "$(get_window_format "$color" "- ")"
tmux set-window-option -t "$target" window-status-current-format "$(get_current_window_format "$color" "- ")"

# Set stopped marker
tmux set-window-option -t "$target" @claude-stopped 1
