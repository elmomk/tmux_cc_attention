#!/usr/bin/env bash
# Called by Claude Code's PreToolUse hook.
# Turns the tmux window label green to indicate Claude is actively working.

[ -z "$TMUX" ] && exit 0
[ -z "$TMUX_PANE" ] && exit 0

CURRENT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$CURRENT_DIR/helpers.sh"

# Consume stdin (Claude Code sends JSON)
cat > /dev/null

target=$(pane_to_window "$TMUX_PANE") || exit 0

# Short-circuit: already active
[ "$(tmux show-window-option -t "$target" -v @claude-active 2>/dev/null)" = "1" ] && exit 0

color=$(get_active_color | tr -cd 'a-zA-Z0-9#')
green_fmt=$(get_window_format "$color")

tmux set-window-option -t "$target" window-status-format "$green_fmt"
tmux set-window-option -t "$target" @claude-active 1
tmux set-window-option -t "$target" -u @claude-stopped 2>/dev/null
tmux set-window-option -t "$target" -u @claude-attention 2>/dev/null
