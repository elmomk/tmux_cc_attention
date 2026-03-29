#!/usr/bin/env bash
# Claude Sessions popup — fzf-based dashboard + quick switcher.
# Runs inside `tmux display-popup -E`. Shows all Claude windows with
# state icons, previews pane content, and switches to the selected window.

if ! command -v fzf >/dev/null 2>&1; then
    echo "fzf is required for the Claude Sessions popup."
    echo "Install it: https://github.com/junegunn/fzf#installation"
    read -r -n 1
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

# Build a set of windows that have a Claude pane (list-panes is authoritative)
declare -A claude_windows
while IFS='|' read -r sess win_idx cmd; do
    if [[ "$cmd" =~ ^[Cc]laude(-code)?$ ]]; then
        claude_windows["${sess}:${win_idx}"]=1
    fi
done < <(tmux list-panes -a -F '#{session_name}|#{window_index}|#{pane_current_command}' 2>/dev/null)

# Collect formatted lines for Claude windows
lines=()
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
        label="${ansi_stop}stopped${ansi_reset}"
    else
        icon=" "
        label="${ansi_dim}no state${ansi_reset}"
    fi

    lines+=("$(printf '%b %-16s %-14s (%b)' "$icon" "$target" "$win_name" "$label")")
done < <(tmux list-windows -a -F '#{session_name}|#{window_index}|#{window_name}|#{@claude-attention}|#{@claude-active}|#{@claude-stopped}' 2>/dev/null)

if [ ${#lines[@]} -eq 0 ]; then
    echo "No Claude sessions found."
    read -r -n 1
    exit 0
fi

# Pipe to fzf
selected=$(printf '%s\n' "${lines[@]}" | fzf --ansi --no-sort \
    --header='Claude Sessions — enter to switch, esc to cancel' \
    --preview='target=$(echo {} | sed "s/^.\{2\}//" | awk "{print \$1}"); tmux capture-pane -ep -t "$target" 2>/dev/null' \
    --preview-window='right:40%')

[ -z "$selected" ] && exit 0

# Extract target (session:window) from the selected line
target=$(echo "$selected" | sed 's/^.\{2\}//' | awk '{print $1}')
[ -n "$target" ] && tmux switch-client -t "$target" 2>/dev/null
