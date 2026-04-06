#!/usr/bin/env bash
# Claude Sessions popup — fzf-based dashboard + quick switcher.
# Groups windows by session with headers. Shows state icons and
# previews pane content. Switches to selected window on enter.

if ! command -v fzf >/dev/null 2>&1; then
    echo "fzf is required for the Claude Sessions popup."
    echo "Install it: https://github.com/junegunn/fzf#installation"
    sleep 3
    exit 1
fi

# Read theme colors for ANSI output
att_color=$(tmux show-option -gqv @claude-attention-color)
att_color="${att_color:-#c4746e}"
act_color=$(tmux show-option -gqv @claude-active-color)
act_color="${act_color:-#87a987}"
stop_color=$(tmux show-option -gqv @claude-stopped-color)
stop_color="${stop_color:-#8ba4b0}"

# Convert hex (#rrggbb) to ANSI 24-bit escape
hex_to_ansi() {
    local hex="${1#\#}"
    printf '\033[38;2;%d;%d;%dm' "0x${hex:0:2}" "0x${hex:2:2}" "0x${hex:4:2}"
}

ansi_att=$(hex_to_ansi "$att_color")
ansi_act=$(hex_to_ansi "$act_color")
ansi_stop=$(hex_to_ansi "$stop_color")
ansi_reset=$'\033[0m'
ansi_dim=$'\033[2m'
ansi_bold=$'\033[1m'

# Build a set of windows that have a Claude pane (list-panes is authoritative)
declare -A claude_windows
while IFS='|' read -r sess win_idx cmd; do
    if [[ "$cmd" =~ ^[Cc]laude(-code)?$ ]]; then
        claude_windows["${sess}:${win_idx}"]=1
    fi
done < <(tmux list-panes -a -F '#{session_name}|#{window_index}|#{pane_current_command}' 2>/dev/null)

# Collect windows grouped by session
declare -A session_lines
session_order=()
while IFS='|' read -r sess win_idx win_name att act stop; do
    target="${sess}:${win_idx}"
    [ -z "${claude_windows[$target]+_}" ] && continue

    if [ "$att" = "1" ]; then
        icon="${ansi_att}!${ansi_reset}"
        label="${ansi_att}needs attention${ansi_reset}"
    elif [ "$act" = "1" ]; then
        icon="${ansi_act}*${ansi_reset}"
        label="${ansi_act}working${ansi_reset}"
    elif [ "$stop" = "1" ]; then
        icon="${ansi_stop}-${ansi_reset}"
        label="${ansi_stop}idle${ansi_reset}"
    else
        icon=" "
        label="${ansi_dim}no state${ansi_reset}"
    fi

    line="$(printf '%b %-18s %-14s (%b)' "$icon" "$target" "$win_name" "$label")"

    if [ -z "${session_lines[$sess]+_}" ]; then
        session_order+=("$sess")
    fi
    session_lines[$sess]+="${line}"$'\n'
done < <(tmux list-windows -a -F '#{session_name}|#{window_index}|#{window_name}|#{@claude-attention}|#{@claude-active}|#{@claude-stopped}' 2>/dev/null)

if [ ${#session_order[@]} -eq 0 ]; then
    echo "No Claude sessions found."
    sleep 2
    exit 0
fi

# Build output with session headers (non-selectable in fzf via --delimiter trick)
output=""
for sess in "${session_order[@]}"; do
    count=$(echo -n "${session_lines[$sess]}" | grep -c '^')
    output+="${ansi_bold}${ansi_dim}── ${sess} (${count}) ──${ansi_reset}"$'\n'
    while IFS= read -r line; do
        [ -z "$line" ] && continue
        output+="  ${line}"$'\n'
    done <<< "${session_lines[$sess]}"
done

# Strip ANSI escapes to extract session:window target
strip_ansi='s/\x1b\[[0-9;]*m//g'
extract_target='sed "s/^[[:space:]]*//" | awk "{print \$1}"'

# Pipe to fzf
selected=$(printf '%s' "$output" | fzf --ansi --no-sort \
    --header='Claude Sessions — enter to switch, esc to cancel' \
    --preview="target=\$(echo {} | sed '$strip_ansi' | $extract_target); tmux capture-pane -ep -t \"\$target\" 2>/dev/null" \
    --preview-window='right:40%')

[ -z "$selected" ] && exit 0

# Extract target (session:window) — strip ANSI, trim whitespace, take first field
target=$(echo "$selected" | sed "$strip_ansi" | sed 's/^[[:space:]]*//' | awk '{print $1}')
# Only switch if target looks like session:window (skip header lines)
[[ "$target" == *:* ]] && tmux switch-client -t "$target" 2>/dev/null
