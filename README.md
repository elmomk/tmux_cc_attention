# tmux_cc_attention

A tmux plugin that provides visual indicators for Claude Code session state. Windows turn **green** when Claude is working, **red** when it needs attention, and **dark** when it stops — with cross-session awareness and bundled color themes.

## Features

- **Active indicator**: Window turns green when Claude Code is working
- **Attention indicator**: Window turns red when Claude Code needs your input
- **Stopped indicator**: Window goes dark when Claude Code stops/idles
- **Auto-clear**: Indicators clear when you switch to the window
- **Cross-session**: Status bar shows indicators from other tmux sessions
- **Themes**: Bundled themes — Kanagawa Dragon, Catppuccin Mocha, Tokyo Night, Dracula

## Installation

1. Clone the repo:
   ```bash
   git clone https://github.com/youruser/tmux_cc_attention ~/tmux_cc_attention
   ```

2. Source a theme and the plugin in `~/.tmux.conf`:
   ```tmux
   # Theme first, then TPM, then plugin last
   source-file '~/tmux_cc_attention/themes/kanagawa-dragon.conf'
   run '~/.tmux/plugins/tpm/tpm'
   run-shell '~/tmux_cc_attention/claude-attention.tmux'
   ```

3. Install the Claude Code hooks:
   ```bash
   ~/tmux_cc_attention/scripts/setup.sh --apply
   ```

4. Reload tmux:
   ```bash
   tmux source ~/.tmux.conf
   ```

## How it works

```
Claude Code PreToolUse hook   → active.sh  → window turns GREEN
Claude Code Notification hook → notify.sh  → window turns RED
Claude Code Stop hook         → stopped.sh → window turns DARK
User switches to window       → clear.sh   → window reverts to normal
tmux status-right             → status.sh  → cross-session indicators
```

State is stored in tmux window options (`@claude-active`, `@claude-attention`, `@claude-stopped`). No temp files.

Priority: **attention (red) > active (green) > stopped (dark) > normal**

## Themes

Swap one line in `~/.tmux.conf` to change theme:

```tmux
source-file '~/tmux_cc_attention/themes/kanagawa-dragon.conf'
# source-file '~/tmux_cc_attention/themes/catppuccin-mocha.conf'
# source-file '~/tmux_cc_attention/themes/tokyonight.conf'
# source-file '~/tmux_cc_attention/themes/dracula.conf'
```

Each theme sets matching colors for the plugin indicators automatically. The selected window tab uses the theme's purple/accent color; green is reserved for active Claude sessions.

## Configuration

| Option | Default | Purpose |
|--------|---------|---------|
| `@claude-active-color` | theme green | Foreground color for active Claude windows |
| `@claude-attention-color` | theme red | Foreground color for attention windows |
| `@claude-stopped-color` | theme bg | Foreground color for stopped windows (blends in) |
| `@claude-attention-bell` | `off` | Also trigger tmux bell on notification |
| `@claude-window-bg` | theme surface | Background color for window labels |

Defaults are set by the theme. Override in `~/.tmux.conf` before sourcing the plugin:

```tmux
set -g @claude-attention-bell "on"
```

## Manual testing

```bash
# Simulate active (from a different window)
echo '{}' | TMUX_PANE=%2 /path/to/scripts/active.sh

# Simulate notification
echo '{}' | TMUX_PANE=%2 /path/to/scripts/notify.sh

# Simulate stop
echo '{}' | TMUX_PANE=%2 /path/to/scripts/stopped.sh

# Verify hooks (should show exactly one entry per hook)
tmux show-hooks -g | grep 'session-window-changed\[100\]'
tmux show-hooks -g | grep 'client-session-changed\[100\]'
```

## License

AGPL-3.0
