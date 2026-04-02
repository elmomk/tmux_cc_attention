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

# -- Clean up old hooks --
for i in 0 1 2 3 4 5; do
    tmux set-hook -gu "session-window-changed[$i]" 2>/dev/null || true
    tmux set-hook -gu "client-session-changed[$i]" 2>/dev/null || true
done

# -- Auto-clear attention/stopped markers when user focuses a window --
CLEAR_ON_FOCUS="$CURRENT_DIR/scripts/clear-on-focus.sh"
tmux set-hook -g "session-window-changed[100]" "run-shell -b '$CLEAR_ON_FOCUS'"
tmux set-hook -g "client-session-changed[100]" "run-shell -b '$CLEAR_ON_FOCUS'"

# Store plugin path so other scripts can find it
tmux set-option -g @claude-attention-plugin-path "$CURRENT_DIR"

# -- Status-right: push-based cross-session counts + periodic stale cleanup --
current_right=$(tmux show-option -gqv status-right)
case "$current_right" in
    *claude-cross-counts*|*status.sh*)
        ;;
    *)
        # #{@claude-cross-counts} is updated instantly by state-change scripts.
        # #($STATUS_SCRIPT) runs at status-interval for stale marker cleanup only (outputs nothing).
        tmux set-option -g status-right "${current_right} #{@claude-cross-counts}#{@claude-done-msg}#($STATUS_SCRIPT)"
        ;;
esac

# -- Opt-in popup keybinding --
popup_key=$(tmux show-option -gqv @claude-popup-key)
if [ -n "$popup_key" ]; then
    tmux bind-key "$popup_key" display-popup -E -w 60% -h 60% -T ' Claude Sessions ' "$CURRENT_DIR/scripts/popup.sh"
fi

# -- Opt-in window auto-naming (names new windows after cwd basename) --
auto_name=$(tmux show-option -gqv @claude-auto-name)
if [ "$auto_name" = "on" ]; then
    AUTO_NAME_SCRIPT="$CURRENT_DIR/scripts/auto-name.sh"
    tmux set-hook -g "after-new-window[100]" "run-shell -b '$AUTO_NAME_SCRIPT'"
    tmux set-hook -g "after-split-window[100]" "run-shell -b '$AUTO_NAME_SCRIPT'"
fi

# -- Opt-in prefix-less navigation --
nav_keys=$(tmux show-option -gqv @claude-nav-keys)
if [ "$nav_keys" = "on" ]; then
    # M-1..9: direct window switching (no prefix needed)
    for i in 1 2 3 4 5 6 7 8 9; do
        tmux bind-key -n "M-$i" select-window -t ":$i"
    done
    tmux bind-key -n M-0 select-window -t ":10"

    # M-H / M-L: previous/next session
    tmux bind-key -n M-H switch-client -p
    tmux bind-key -n M-L switch-client -n

    # M-h / M-l: previous/next window
    tmux bind-key -n M-h previous-window
    tmux bind-key -n M-l next-window

    # M-n: new window (in current path)
    tmux bind-key -n M-n new-window -c "#{pane_current_path}"
fi
