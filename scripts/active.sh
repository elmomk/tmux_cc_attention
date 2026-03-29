#!/usr/bin/env bash
# Called by Claude Code's PreToolUse hook.
# Turns the tmux window label green to indicate Claude is actively working.

[ -z "$TMUX" ] && exit 0
[ -z "$TMUX_PANE" ] && exit 0

# Drain stdin in background (polite to Claude Code, non-blocking for us)
cat > /dev/null &

# Single tmux call: get target + current markers (pipe-delimited)
info=$(tmux display-message -p -t "$TMUX_PANE" \
    '#{session_name}:#{window_index}|#{@claude-active}|#{@claude-attention}')
target="${info%%|*}"
rest="${info#*|}"
current_active="${rest%%|*}"
current_attention="${rest##*|}"

# Short-circuit: already active, or attention is set (never overwrite red)
[ "$current_active" = "1" ] && exit 0
[ "$current_attention" = "1" ] && exit 0

# Read cached format strings (precomputed at plugin load, 1 tmux call)
fmt=$(tmux show-option -gqv @claude-fmt-active)
fmt_cur=$(tmux show-option -gqv @claude-fmt-active-cur)

tmux set-window-option -t "$target" window-status-format "$fmt" 2>/dev/null
tmux set-window-option -t "$target" window-status-current-format "$fmt_cur" 2>/dev/null
tmux set-window-option -t "$target" @claude-active 1 2>/dev/null
tmux set-window-option -t "$target" -u @claude-stopped 2>/dev/null

# Guard against race: if notify.sh set attention while we were writing,
# restore the attention format so the window doesn't stay green.
if [ "$(tmux show-window-option -t "$target" -v @claude-attention 2>/dev/null)" = "1" ]; then
    att_fmt=$(tmux show-option -gqv @claude-fmt-attention)
    att_fmt_cur=$(tmux show-option -gqv @claude-fmt-attention-cur)
    tmux set-window-option -t "$target" window-status-format "$att_fmt" 2>/dev/null
    tmux set-window-option -t "$target" window-status-current-format "$att_fmt_cur" 2>/dev/null
    tmux set-window-option -t "$target" -u @claude-active 2>/dev/null
fi
