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

# Don't override attention — if Claude asked a question, that's what the user needs to see.
# Notification fires right before Stop when Claude needs input.
[ "$(tmux show-window-option -t "$target" -v @claude-attention 2>/dev/null)" = "1" ] && exit 0

color=$(get_stopped_color | tr -cd 'a-zA-Z0-9#')

tmux set-window-option -t "$target" window-status-format "$(get_window_format "$color" "- ")"
tmux set-window-option -t "$target" window-status-current-format "$(get_current_window_format "$color" "- ")"

# Set stopped marker, clear active
tmux set-window-option -t "$target" @claude-stopped 1
tmux set-window-option -t "$target" -u @claude-active 2>/dev/null
