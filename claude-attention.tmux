#!/usr/bin/env bash
# TPM-compatible entry point for tmux Claude Code Attention plugin.
# Requires tmux >= 3.0 for indexed hooks.
#
# Usage in .tmux.conf:
#   set -g @plugin 'elmomk/tmux_cc_attention'
#   set -g @claude-theme 'kanagawa-dragon'  # optional

CURRENT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

STATUS_SCRIPT="$CURRENT_DIR/scripts/status.sh"

# -- Load theme --
theme=$(tmux show-option -gqv @claude-theme)
theme="${theme:-kanagawa-dragon}"
case "$theme" in
    kanagawa-dragon|catppuccin-mocha|tokyonight|dracula) ;;
    *) theme="kanagawa-dragon" ;;
esac
tmux source-file "$CURRENT_DIR/themes/${theme}.conf"

# -- Clean up old auto-clear hooks --
for i in 0 1 2 3 4 5 100; do
    tmux set-hook -gu "session-window-changed[$i]" 2>/dev/null || true
    tmux set-hook -gu "client-session-changed[$i]" 2>/dev/null || true
done

# Store plugin path so other scripts can find it
tmux set-option -g @claude-attention-plugin-path "$CURRENT_DIR"

# -- Status-right: single cross-session indicator call --
current_right=$(tmux show-option -gqv status-right)
case "$current_right" in
    *status.sh*)
        ;;
    *)
        tmux set-option -g status-right "${current_right} #($STATUS_SCRIPT)"
        ;;
esac
