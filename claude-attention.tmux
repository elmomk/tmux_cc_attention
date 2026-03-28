#!/usr/bin/env bash
# TPM-compatible entry point for tmux Claude Code Attention plugin.
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

# -- Status-right: cross-session indicators --
att_color=$(tmux show-option -gqv @claude-attention-color)
att_color="${att_color:-#c4746e}"
act_color=$(tmux show-option -gqv @claude-active-color)
act_color="${act_color:-#87a987}"
stop_color=$(tmux show-option -gqv @claude-stopped-color)
stop_color="${stop_color:-#282727}"

current_right=$(tmux show-option -gqv status-right)
case "$current_right" in
    *status.sh*)
        ;;
    *)
        tmux set-option -g status-right "${current_right} #[fg=${att_color}]#($STATUS_SCRIPT --attention)#[fg=${act_color}]#($STATUS_SCRIPT --active)#[fg=${stop_color}]#($STATUS_SCRIPT --stopped)"
        ;;
esac
