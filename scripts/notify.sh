#!/usr/bin/env bash
# Called by Claude Code's Notification hook.
# Turns the tmux window label red to signal attention needed.

[ -z "$TMUX" ] && exit 0
[ -z "$TMUX_PANE" ] && exit 0

CURRENT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$CURRENT_DIR/helpers.sh"

# Consume stdin (Claude Code sends JSON)
cat > /dev/null

target=$(pane_to_window "$TMUX_PANE") || exit 0

color=$(get_attention_color | tr -cd 'a-zA-Z0-9#')
red_fmt=$(get_window_format "$color")

tmux set-window-option -t "$target" window-status-format "$red_fmt"

# Set attention marker, clear active/stopped (attention takes priority)
tmux set-window-option -t "$target" @claude-attention 1
tmux set-window-option -t "$target" -u @claude-active 2>/dev/null
tmux set-window-option -t "$target" -u @claude-stopped 2>/dev/null

# Optional bell
bell=$(get_tmux_option "@claude-attention-bell" "off")
if [ "$bell" = "on" ]; then
    tmux run-shell -t "$target" 'printf "\a"'
fi
