#!/usr/bin/env bash
# Called by Claude Code's Notification hook.
# Turns the tmux window label red to signal attention needed.

[ -z "$TMUX" ] && exit 0
[ -z "$TMUX_PANE" ] && exit 0

# Close stdin immediately (no fork, no blocking)
exec 0</dev/null

target=$(tmux display-message -p -t "$TMUX_PANE" '#{session_name}:#{window_index}') || exit 0

# Read cached format strings (precomputed at plugin load)
fmt=$(tmux show-option -gqv @claude-fmt-attention)
fmt_cur=$(tmux show-option -gqv @claude-fmt-attention-cur)

tmux set-window-option -t "$target" window-status-format "$fmt" 2>/dev/null
tmux set-window-option -t "$target" window-status-current-format "$fmt_cur" 2>/dev/null

# Set attention marker. Leave @claude-active so concurrent PreToolUse calls
# hit the short-circuit instead of racing to overwrite red with green.
tmux set-window-option -t "$target" @claude-attention 1 2>/dev/null
tmux set-window-option -t "$target" -u @claude-stopped 2>/dev/null

# Optional bell
bell=$(tmux show-option -gqv @claude-attention-bell)
if [ "$bell" = "on" ]; then
    tmux run-shell -t "$target" 'printf "\a"'
fi
