#!/usr/bin/env bash
# Cross-session status-right component.
# Runs every status-interval (5s). Two jobs in a single list-windows pass:
# 1. Clean up stale markers (Claude Code exited but window still colored)
# 2. Show counts of attention/active/stopped windows from OTHER sessions.

current_session=$(tmux display-message -p '#{session_name}' 2>/dev/null) || exit 0

# Read colors
att_color=$(tmux show-option -gqv @claude-attention-color)
att_color="${att_color:-#c4746e}"
act_color=$(tmux show-option -gqv @claude-active-color)
act_color="${act_color:-#87a987}"
stop_color=$(tmux show-option -gqv @claude-stopped-color)
stop_color="${stop_color:-#8ba4b0}"

att_count=0
act_count=0
stop_count=0

# Single pass over all windows
while IFS='|' read -r sess_name win_idx att act stop; do
    [ "$att" != "1" ] && [ "$act" != "1" ] && [ "$stop" != "1" ] && continue

    target="${sess_name}:${win_idx}"

    # Check if any pane in this window is still running claude
    if ! tmux list-panes -t "$target" -F '#{pane_current_command}' 2>/dev/null | grep -qiE '^claude(-code)?$'; then
        # Re-verify before clearing (close TOCTOU window)
        if ! tmux list-panes -t "$target" -F '#{pane_current_command}' 2>/dev/null | grep -qiE '^claude(-code)?$'; then
            tmux set-window-option -t "$target" -u window-status-format 2>/dev/null
            tmux set-window-option -t "$target" -u window-status-current-format 2>/dev/null
            tmux set-window-option -t "$target" -u @claude-attention 2>/dev/null
            tmux set-window-option -t "$target" -u @claude-active 2>/dev/null
            tmux set-window-option -t "$target" -u @claude-stopped 2>/dev/null
        fi
        continue
    fi

    # Count cross-session markers (skip current session)
    [ "$sess_name" = "$current_session" ] && continue

    if [ "$att" = "1" ]; then
        att_count=$((att_count + 1))
    elif [ "$act" = "1" ]; then
        act_count=$((act_count + 1))
    elif [ "$stop" = "1" ]; then
        stop_count=$((stop_count + 1))
    fi
done < <(tmux list-windows -a -F "#{session_name}|#{window_index}|#{@claude-attention}|#{@claude-active}|#{@claude-stopped}" 2>/dev/null)

output=""
[ "$att_count" -gt 0 ] && output="${output}#[fg=${att_color}]!${att_count} "
[ "$act_count" -gt 0 ] && output="${output}#[fg=${act_color}]*${act_count} "
[ "$stop_count" -gt 0 ] && output="${output}#[fg=${stop_color}]-${stop_count} "

echo -n "$output"
