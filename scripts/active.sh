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
    '#{session_name}:#{window_index} #{@claude-active} #{@claude-attention}')
target="${info%% *}"
rest="${info#* }"
current_active="${rest%% *}"
current_attention="${rest##* }"

# Short-circuit: already active, or attention is set (never overwrite red)
[ "$current_active" = "1" ] && exit 0
[ "$current_attention" = "1" ] && exit 0

color=$(get_active_color | tr -cd 'a-zA-Z0-9#')

tmux set-window-option -t "$target" window-status-format "$(get_window_format "$color" "* ")"
tmux set-window-option -t "$target" window-status-current-format "$(get_current_window_format "$color" "* ")"
tmux set-window-option -t "$target" @claude-active 1
tmux set-window-option -t "$target" -u @claude-stopped 2>/dev/null

# Guard against race: if notify.sh set attention while we were writing,
# restore the attention format so the window doesn't stay green.
if [ "$(tmux show-window-option -t "$target" -v @claude-attention 2>/dev/null)" = "1" ]; then
    att_color=$(get_attention_color | tr -cd 'a-zA-Z0-9#')
    tmux set-window-option -t "$target" window-status-format "$(get_window_format "$att_color" "! ")"
    tmux set-window-option -t "$target" window-status-current-format "$(get_current_window_format "$att_color" "! ")"
    tmux set-window-option -t "$target" -u @claude-active 2>/dev/null
fi
