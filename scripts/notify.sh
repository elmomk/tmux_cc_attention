#!/usr/bin/env bash
# Called by Claude Code's Notification hook.
# Turns the tmux window label red to signal attention needed.

[ -z "$TMUX" ] && exit 0
[ -z "$TMUX_PANE" ] && exit 0

CURRENT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$CURRENT_DIR/helpers.sh"

# Consume stdin (Claude Code sends JSON payload)
cat > /dev/null

target=$(pane_to_window "$TMUX_PANE") || exit 0

color=$(get_attention_color | tr -cd 'a-zA-Z0-9#')

tmux set-window-option -t "$target" window-status-format "$(get_window_format "$color" "! ")" 2>/dev/null
tmux set-window-option -t "$target" window-status-current-format "$(get_current_window_format "$color" "! ")" 2>/dev/null

# Set attention marker. Leave @claude-active so concurrent PreToolUse calls
# hit the short-circuit instead of racing to overwrite red with green.
tmux set-window-option -t "$target" @claude-attention 1 2>/dev/null
tmux set-window-option -t "$target" -u @claude-stopped 2>/dev/null

# Optional bell
bell=$(get_tmux_option "@claude-attention-bell" "off")
if [ "$bell" = "on" ]; then
    tmux run-shell -t "$target" 'printf "\a"'
fi
