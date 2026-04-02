#!/usr/bin/env bash
# Auto-name new windows based on the pane's working directory basename.
# Runs via after-new-window hook. Only renames if the window still has
# the default name (bash/zsh/fish) — respects manual renames.

target=$(tmux display-message -p '#{session_name}:#{window_index}|#{window_name}|#{pane_current_path}') || exit 0

IFS='|' read -r tgt win_name pane_path <<< "$target"

# Only auto-name if the window has a default shell name
case "$win_name" in
    bash|zsh|fish|sh|ksh|tcsh) ;;
    *) exit 0 ;;
esac

# Use the directory basename as the window name
dir_name=$(basename "$pane_path" 2>/dev/null)
[ -z "$dir_name" ] && exit 0

# Shorten home directory to ~
if [ "$dir_name" = "$(basename "$HOME")" ] && [ "$pane_path" = "$HOME" ]; then
    dir_name="~"
fi

tmux rename-window -t "$tgt" "$dir_name" 2>/dev/null
