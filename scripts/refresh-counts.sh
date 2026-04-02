#!/usr/bin/env bash
# Push-based cross-session count updater.
# Called by state-change scripts (active/notify/stopped/clear-on-focus)
# to instantly update the cross-session indicator in status-right.
# Also performs stale marker cleanup in the same pass.

current_session=$(tmux display-message -p '#{session_name}' 2>/dev/null) || exit 0

# Single pass: all marked windows + pane commands together
all_windows=$(tmux list-windows -a -F '#{session_name}|#{window_index}|#{@claude-attention}|#{@claude-active}|#{@claude-stopped}' 2>/dev/null)

[ -z "$all_windows" ] && { tmux set-option -g @claude-cross-counts "" 2>/dev/null; exit 0; }

# Early exit: no markers anywhere
if ! grep -qE '\|1' <<< "$all_windows"; then
    tmux set-option -g @claude-cross-counts "" 2>/dev/null
    exit 0
fi

# Build claude pane lookup (single list-panes call)
claude_set=$(tmux list-panes -a -F '#{session_name}:#{window_index}|#{pane_current_command}' 2>/dev/null \
    | grep -iE 'claude(-code)?$' | cut -d'|' -f1 | sort -u)

# Read colors once
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

    # Stale cleanup: Claude no longer running in this window
    if ! grep -qF "$target" <<< "$claude_set"; then
        tmux set-window-option -t "$target" -u window-status-format \; \
             set-window-option -t "$target" -u window-status-current-format \; \
             set-window-option -t "$target" -u @claude-attention \; \
             set-window-option -t "$target" -u @claude-active \; \
             set-window-option -t "$target" -u @claude-stopped 2>/dev/null
        continue
    fi

    # Count cross-session markers only
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

# Write to global option — status-right reads #{@claude-cross-counts} instantly
tmux set-option -g @claude-cross-counts "$output" 2>/dev/null
