#!/usr/bin/env bash
# Called by Claude Code's Stop hook.
# Turns the tmux window label black/dark to indicate Claude has stopped.

[ -z "$TMUX" ] && exit 0
[ -z "$TMUX_PANE" ] && exit 0

CURRENT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$CURRENT_DIR/helpers.sh"

# Consume stdin (Claude Code sends JSON)
cat > /dev/null

target=$(pane_to_window "$TMUX_PANE") || exit 0

# Skip if attention is already set (attention takes priority)
[ "$(tmux show-window-option -t "$target" -v @claude-attention 2>/dev/null)" = "1" ] && exit 0

color=$(get_stopped_color | tr -cd 'a-zA-Z0-9#')
dark_fmt=$(get_window_format "$color")

tmux set-window-option -t "$target" window-status-format "$dark_fmt"

# Set stopped marker, clear active
tmux set-window-option -t "$target" @claude-stopped 1
tmux set-window-option -t "$target" -u @claude-active 2>/dev/null
