#!/usr/bin/env bash
# Called by Claude Code's Stop hook.
# Turns the tmux window label blue to indicate Claude has stopped.

[ -z "$TMUX" ] && exit 0
[ -z "$TMUX_PANE" ] && exit 0

# Drain stdin in background (polite to Claude Code, non-blocking for us)
cat > /dev/null &

info=$(tmux display-message -p -t "$TMUX_PANE" \
    '#{session_name}:#{window_index}|#{window_name}|#{@claude-active}') || exit 0
target="${info%%|*}"
rest="${info#*|}"
win_name="${rest%%|*}"
was_active="${rest##*|}"

# Read cached format strings (precomputed at plugin load)
fmt=$(tmux show-option -gqv @claude-fmt-stopped)
fmt_cur=$(tmux show-option -gqv @claude-fmt-stopped-cur)

tmux set-window-option -t "$target" window-status-format "$fmt" 2>/dev/null
tmux set-window-option -t "$target" window-status-current-format "$fmt_cur" 2>/dev/null

# Set stopped marker, clear active and attention
tmux set-window-option -t "$target" @claude-stopped 1 2>/dev/null
tmux set-window-option -t "$target" -u @claude-active 2>/dev/null
tmux set-window-option -t "$target" -u @claude-attention 2>/dev/null
tmux set-window-option -t "$target" -u @claude-idle 2>/dev/null

# Opt-in done notification: brief inline status-right indicator on active → stopped
if [ "$was_active" = "1" ]; then
    done_popup=$(tmux show-option -gqv @claude-done-popup)
    if [ "$done_popup" = "on" ]; then
        stop_color=$(tmux show-option -gqv @claude-stopped-color)
        stop_color="${stop_color:-#8ba4b0}"
        tmux set-option -g @claude-done-msg "#[fg=${stop_color}]done:${target} "
        # Auto-clear after 5 seconds in background
        (sleep 5 && tmux set-option -gu @claude-done-msg 2>/dev/null) &
        disown
    fi
fi
