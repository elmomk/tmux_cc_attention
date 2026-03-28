# tmux Claude Code Attention Plugin

A tmux plugin that provides visual indicators when Claude Code needs your attention or has stopped working. Windows turn **red** on notifications and **grey** when Claude stops, with cross-session awareness so you never miss a prompt.

## Features

- **Attention indicator**: Window turns red when Claude Code sends a notification
- **Stopped indicator**: Window turns grey when Claude Code stops/idles
- **Auto-clear**: Indicators clear when you switch to the window
- **Cross-session**: Status bar shows attention/stopped windows from other sessions
- **Custom theme**: Dracula-palette theme designed to integrate cleanly (optional)

## Installation

### Manual

1. Clone the repo:
   ```bash
   git clone https://github.com/youruser/tmux_claude_code_plugin ~/tmux_claude_code_plugin
   ```

2. Source the theme (optional) and plugin in `~/.tmux.conf`:
   ```tmux
   source-file '~/tmux_claude_code_plugin/theme.conf'
   run-shell '~/tmux_claude_code_plugin/claude-attention.tmux'
   ```

3. Install the Claude Code hooks:
   ```bash
   ~/tmux_claude_code_plugin/scripts/setup.sh --apply
   ```

### Load order

If using the bundled theme with TPM:
```tmux
source-file '~/tmux_claude_code_plugin/theme.conf'   # Theme first
run '~/.tmux/plugins/tpm/tpm'                         # TPM plugins
run-shell '~/tmux_claude_code_plugin/claude-attention.tmux'  # Plugin last
```

## Configuration

| Option | Default | Purpose |
|--------|---------|---------|
| `@claude-attention-color` | `#ff5555` | Foreground color for attention windows |
| `@claude-stopped-color` | `#6272a4` | Foreground color for stopped/idle windows |
| `@claude-attention-bell` | `off` | Also trigger tmux bell on notification |
| `@claude-window-bg` | `#44475a` | Background color for window labels |

Set options in `~/.tmux.conf` before sourcing the plugin:
```tmux
set -g @claude-attention-color "#ff5555"
set -g @claude-attention-bell "on"
```

## How it works

```
Claude Code Notification hook → notify.sh  → window turns RED
Claude Code Stop hook         → stopped.sh → window turns GREY
User switches to window       → clear.sh   → window reverts to normal
tmux status-right             → status.sh  → shows cross-session indicators
```

State is stored in tmux window options (`@claude-attention`, `@claude-stopped`). No temp files. Priority: attention (red) > stopped (grey) > normal.

## Manual testing

```bash
# Simulate notification (from a different window)
echo '{}' | TMUX_PANE=%2 /path/to/scripts/notify.sh

# Simulate stop
echo '{}' | TMUX_PANE=%2 /path/to/scripts/stopped.sh

# Verify hooks (should show exactly one entry per hook)
tmux show-hooks -g | grep 'session-window-changed\[100\]'
tmux show-hooks -g | grep 'client-session-changed\[100\]'
```

## License

MIT
# tmux_cc_attention
