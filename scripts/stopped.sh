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

color=$(get_stopped_color | tr -cd 'a-zA-Z0-9#')

tmux set-window-option -t "$target" window-status-format "$(get_window_format "$color" "- ")" 2>/dev/null
tmux set-window-option -t "$target" window-status-current-format "$(get_current_window_format "$color" "- ")" 2>/dev/null

# Set stopped marker, clear active and attention
tmux set-window-option -t "$target" @claude-stopped 1 2>/dev/null
tmux set-window-option -t "$target" -u @claude-active 2>/dev/null
tmux set-window-option -t "$target" -u @claude-attention 2>/dev/null
