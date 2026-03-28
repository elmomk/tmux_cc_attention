#!/usr/bin/env bash

get_tmux_option() {
    local option="$1"
    local default_value="$2"
    local value
    value=$(tmux show-option -gqv "$option")
    if [ -n "$value" ]; then
        echo "$value"
    else
        echo "$default_value"
    fi
}

get_attention_color() {
    get_tmux_option "@claude-attention-color" "#c4746e"
}

get_active_color() {
    get_tmux_option "@claude-active-color" "#87a987"
}

get_stopped_color() {
    get_tmux_option "@claude-stopped-color" "#8ba4b0"
}

# Build window-status-format with state color and prefix icon
# Usage: get_window_format <fg_color> [prefix]
get_window_format() {
    local fg="$1"
    local prefix="${2:-}"
    local bg
    bg=$(get_tmux_option "@claude-window-bg" "#282727" | tr -cd 'a-zA-Z0-9#')
    echo "#[fg=${fg},bg=${bg}] ${prefix}#I #W "
}

# Build window-status-current-format with state color as background
# Usage: get_current_window_format <color> [prefix]
get_current_window_format() {
    local color="$1"
    local prefix="${2:-}"
    local bg
    bg=$(get_tmux_option "@claude-window-bg" "#282727" | tr -cd 'a-zA-Z0-9#')
    echo "#[fg=${bg},bg=${color},bold] ${prefix}#I #W "
}

pane_to_window() {
    local pane_id="$1"
    tmux display-message -p -t "$pane_id" '#{session_name}:#{window_index}'
}
