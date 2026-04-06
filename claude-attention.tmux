#!/usr/bin/env bash
# TPM-compatible entry point for tmux Claude Code Attention plugin.
# Requires tmux >= 3.0 for indexed hooks.
#
# Usage in .tmux.conf:
#   set -g @plugin 'elmomk/tmux_cc_attention'
#   set -g @claude-theme 'kanagawa-dragon'  # optional

CURRENT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLAUDE_STATE="$CURRENT_DIR/bin/claude-state"

# Ensure binary exists: check → download prebuilt → build from source
if [ ! -x "$CLAUDE_STATE" ]; then
    mkdir -p "$CURRENT_DIR/bin"
    _os=$(uname -s | tr '[:upper:]' '[:lower:]')
    _arch=$(uname -m)
    case "$_arch" in
        x86_64)  _arch="amd64" ;;
        aarch64) _arch="arm64" ;;
    esac
    _url="https://github.com/elmomk/tmux_cc_attention/releases/latest/download/claude-state-${_os}-${_arch}"

    if command -v curl >/dev/null 2>&1; then
        curl -fsSL -o "$CLAUDE_STATE" "$_url" 2>/dev/null && chmod +x "$CLAUDE_STATE"
    fi

    # Fallback: build from source if download failed
    if [ ! -x "$CLAUDE_STATE" ] && command -v go >/dev/null 2>&1; then
        (cd "$CURRENT_DIR" && go build -ldflags="-s -w" -o bin/claude-state ./cmd/claude-state/) 2>/dev/null
    fi
fi

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
tmux set-hook -g "session-window-changed[100]" "run-shell -b '$CLAUDE_STATE clear'"
tmux set-hook -g "client-session-changed[100]" "run-shell -b '$CLAUDE_STATE clear'"

# Store plugin path so other scripts can find it
tmux set-option -g @claude-attention-plugin-path "$CURRENT_DIR"

# -- Status-right: cross-session counts (push-updated by daemon) --
current_right=$(tmux show-option -gqv status-right)
# Strip legacy status.sh #() calls from previous plugin versions
current_right=$(echo "$current_right" | sed 's|#([^)]*status\.sh)||g')
case "$current_right" in
    *claude-cross-counts*)
        ;;
    *)
        tmux set-option -g status-right "${current_right} #{@claude-watcher}#{@claude-cross-counts}#{@claude-done-msg}"
        ;;
esac

# -- Start state daemon (if binary exists) --
if [ -x "$CLAUDE_STATE" ]; then
    "$CLAUDE_STATE" serve &
    disown
fi

# -- Opt-in popup keybinding --
popup_key=$(tmux show-option -gqv @claude-popup-key)
if [ -n "$popup_key" ]; then
    tmux bind-key "$popup_key" display-popup -E -w 80% -h 80% -T ' Claude Sessions ' "$CURRENT_DIR/scripts/popup.sh"
    tmux bind-key -n "M-g" display-popup -E -w 80% -h 80% -T ' Claude Sessions ' "$CURRENT_DIR/scripts/popup.sh"
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
    tmux bind-key -n M-0 select-window -t ":0"

    # M-H / M-L: previous/next session
    tmux bind-key -n M-H switch-client -p
    tmux bind-key -n M-L switch-client -n

    # M-h / M-l: previous/next window
    tmux bind-key -n M-h previous-window
    tmux bind-key -n M-l next-window

    # M-n: new window (in current path)
    tmux bind-key -n M-n new-window -c "#{pane_current_path}"
fi
