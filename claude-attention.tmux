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

# -- Precompute and cache format strings (eliminates IPC calls in hot path) --
att_color=$(tmux show-option -gqv @claude-attention-color)
att_color="${att_color:-#c4746e}"
act_color=$(tmux show-option -gqv @claude-active-color)
act_color="${act_color:-#87a987}"
stop_color=$(tmux show-option -gqv @claude-stopped-color)
stop_color="${stop_color:-#8ba4b0}"
win_bg=$(tmux show-option -gqv @claude-window-bg)
win_bg="${win_bg:-#282727}"

tmux set-option -g @claude-fmt-active       "#[fg=${act_color},bg=${win_bg}] * #I #W "
tmux set-option -g @claude-fmt-active-cur   "#[fg=${win_bg},bg=${act_color},bold] * #I #W "
tmux set-option -g @claude-fmt-attention    "#[fg=${att_color},bg=${win_bg}] ! #I #W "
tmux set-option -g @claude-fmt-attention-cur "#[fg=${win_bg},bg=${att_color},bold] ! #I #W "
tmux set-option -g @claude-fmt-stopped      "#[fg=${stop_color},bg=${win_bg}] - #I #W "
tmux set-option -g @claude-fmt-stopped-cur  "#[fg=${win_bg},bg=${stop_color},bold] - #I #W "

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

# -- Opt-in popup keybinding --
popup_key=$(tmux show-option -gqv @claude-popup-key)
if [ -n "$popup_key" ]; then
    tmux bind-key "$popup_key" display-popup -E -w 60% -h 60% -T ' Claude Sessions ' "$CURRENT_DIR/scripts/popup.sh"
fi
