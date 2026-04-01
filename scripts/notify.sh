#!/usr/bin/env bash
# Called by Claude Code's Notification hook.
# Turns the tmux window label red to signal attention needed.

[ -z "$TMUX" ] && exit 0
[ -z "$TMUX_PANE" ] && exit 0

# Drain stdin in background (polite to Claude Code, non-blocking for us)
cat > /dev/null &

target=$(tmux display-message -p -t "$TMUX_PANE" '#{session_name}:#{window_index}') || exit 0

# Read cached format strings (precomputed at plugin load)
fmt=$(tmux show-option -gqv @claude-fmt-attention)
fmt_cur=$(tmux show-option -gqv @claude-fmt-attention-cur)

tmux set-window-option -t "$target" window-status-format "$fmt" \; \
     set-window-option -t "$target" window-status-current-format "$fmt_cur" \; \
     set-window-option -t "$target" @claude-attention 1 \; \
     set-window-option -t "$target" -u @claude-active \; \
     set-window-option -t "$target" -u @claude-stopped 2>/dev/null

# Optional bell
bell=$(tmux show-option -gqv @claude-attention-bell)
if [ "$bell" = "on" ]; then
    tmux run-shell -t "$target" 'printf "\a"'
fi

# Optional desktop notification
desktop=$(tmux show-option -gqv @claude-attention-desktop)
if [ "$desktop" = "on" ]; then
    if command -v notify-send &>/dev/null; then
        notify-send -u normal "Claude needs input" "$target"
    elif command -v osascript &>/dev/null; then
        osascript -e "display notification \"$target\" with title \"Claude needs input\""
    fi
fi
