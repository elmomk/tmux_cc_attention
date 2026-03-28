#!/usr/bin/env bash
# Called by Claude Code's PreToolUse hook.
# Turns the tmux window label green to indicate Claude is actively working.

[ -z "$TMUX" ] && exit 0
[ -z "$TMUX_PANE" ] && exit 0

CURRENT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$CURRENT_DIR/helpers.sh"

# Consume stdin (Claude Code sends JSON payload)
cat > /dev/null

# Single tmux call: get target + current markers
info=$(tmux display-message -p -t "$TMUX_PANE" \
    '#{session_name}:#{window_index} #{@claude-active}')
target="${info%% *}"
current_active="${info##* }"

# Short-circuit: already active
[ "$current_active" = "1" ] && exit 0

color=$(get_active_color | tr -cd 'a-zA-Z0-9#')

tmux set-window-option -t "$target" window-status-format "$(get_window_format "$color" "* ")"
tmux set-window-option -t "$target" window-status-current-format "$(get_current_window_format "$color" "* ")"
tmux set-window-option -t "$target" @claude-active 1
tmux set-window-option -t "$target" -u @claude-stopped 2>/dev/null
tmux set-window-option -t "$target" -u @claude-attention 2>/dev/null
