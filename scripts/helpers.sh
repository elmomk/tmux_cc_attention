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
    get_tmux_option "@claude-stopped-color" "#282727"
}

get_window_format() {
    local fg="$1"
    local bg
    bg=$(get_tmux_option "@claude-window-bg" "#282727")
    echo "#[fg=${fg},bg=${bg}] #I #W "
}

pane_to_window() {
    local pane_id="$1"
    tmux display-message -p -t "$pane_id" '#{session_name}:#{window_index}'
}

# Check if the pane's window is currently the active (viewed) window
is_window_active() {
    local pane_id="$1"
    [ "$(tmux display-message -p -t "$pane_id" '#{window_active}')" = "1" ]
}
