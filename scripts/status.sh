#!/usr/bin/env bash
# Cross-session status-right component.
# Runs every status-interval (5s). Two jobs in a single list-windows pass:
# 1. Clean up stale markers (Claude Code exited but window still colored)
# 2. Show counts of attention/active/stopped windows from OTHER sessions.

current_session=$(tmux display-message -p '#{session_name}' 2>/dev/null) || exit 0

# Collect all marked windows in one pass
all_windows=$(tmux list-windows -a -F '#{session_name}|#{window_index}|#{@claude-attention}|#{@claude-active}|#{@claude-stopped}' 2>/dev/null)

# Early exit: no windows at all
[ -z "$all_windows" ] && exit 0

# Check if any window has markers — skip everything if none do
if ! grep -qE '\|1' <<< "$all_windows"; then
    exit 0
fi

# Build lookup set of windows running Claude (single list-panes -a call)
claude_set=$(tmux list-panes -a -F '#{session_name}:#{window_index}|#{pane_current_command}' 2>/dev/null \
    | grep -iE 'claude(-code)?$' | cut -d'|' -f1 | sort -u)

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

while IFS='|' read -r sess_name win_idx att act stop; do
    [ "$att" != "1" ] && [ "$act" != "1" ] && [ "$stop" != "1" ] && continue

    target="${sess_name}:${win_idx}"

    # Check if Claude is still running in this window (lookup, no subprocess)
    if ! grep -qF "$target" <<< "$claude_set"; then
        tmux set-window-option -t "$target" -u window-status-format \; \
             set-window-option -t "$target" -u window-status-current-format \; \
             set-window-option -t "$target" -u @claude-attention \; \
             set-window-option -t "$target" -u @claude-active \; \
             set-window-option -t "$target" -u @claude-stopped 2>/dev/null
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
done <<< "$all_windows"

output=""
[ "$att_count" -gt 0 ] && output="${output}#[fg=${att_color}]!${att_count} "
[ "$act_count" -gt 0 ] && output="${output}#[fg=${act_color}]*${act_count} "
[ "$stop_count" -gt 0 ] && output="${output}#[fg=${stop_color}]-${stop_count} "

echo -n "$output"
