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

# A window name is expanded by tmux as a format string in the status line, so a
# directory named e.g. '#(cmd)' would execute. Restrict to a safe charset (keep
# ~ for the home case), collapse to <=3 words, and cap the length — same rule as
# tmux-claude-name's sanitizer.
dir_name=$(printf '%s' "$dir_name" \
    | tr -cd 'A-Za-z0-9 ._/~-' \
    | tr -s ' ' \
    | cut -d' ' -f1-3 \
    | cut -c1-24)
[ -z "$dir_name" ] && exit 0

tmux rename-window -t "$tgt" "$dir_name" 2>/dev/null
