#!/usr/bin/env bash
# Cross-session status-right component.
# Shows windows with attention/active/stopped markers from OTHER sessions.
# Usage: status.sh --attention | status.sh --active | status.sh --stopped

mode="${1:---attention}"
current_session=$(tmux display-message -p '#{session_name}' 2>/dev/null) || exit 0

# Single call: get all windows with session name, window index, and marker values
results=$(tmux list-windows -a -F "#{session_name}:#{window_index} #{session_name} #{@claude-attention} #{@claude-active} #{@claude-stopped}" 2>/dev/null)

output=""
while IFS=' ' read -r win_target sess_name att_val act_val stop_val; do
    # Skip windows in the current session (those use window colors directly)
    [ "$sess_name" = "$current_session" ] && continue

    case "$mode" in
        --attention)
            [ "$att_val" = "1" ] && output="${output:+${output} }${win_target}"
            ;;
        --active)
            [ "$act_val" = "1" ] && [ "$att_val" != "1" ] && output="${output:+${output} }${win_target}"
            ;;
        --stopped)
            [ "$stop_val" = "1" ] && [ "$att_val" != "1" ] && output="${output:+${output} }${win_target}"
            ;;
    esac
done <<< "$results"

echo -n "$output"
