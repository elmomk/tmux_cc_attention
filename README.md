# tmux_cc_attention

A tmux plugin that provides visual indicators for Claude Code session state. Windows show `*` green when Claude is working, `!` red when it needs attention, and `-` blue when it stops — with cross-session awareness and bundled color themes.

Requires **tmux >= 3.0**.

## Features

- **Active indicator**: `* green` — Claude Code is working
- **Attention indicator**: `! red` — Claude Code needs your input
- **Stopped indicator**: `- blue` — Claude Code has stopped
- **Persistent state**: Colors stay until the actual state changes
- **Current window**: State visible even on the selected window tab
- **Cross-session**: Status bar shows `!3 *2 -1` counts from other sessions
- **Colorblind-friendly**: Text prefixes (`*`, `!`, `-`) alongside colors
- **Themes**: Kanagawa Dragon, Catppuccin Mocha, Tokyo Night, Dracula

## Installation

### Via TPM

Add to `~/.tmux.conf`:

```tmux
set -g @claude-theme 'catppuccin-mocha'  # or kanagawa-dragon, tokyonight, dracula
set -g @plugin 'elmomk/tmux_cc_attention'

run '~/.tmux/plugins/tpm/tpm'
```

Then press `prefix + I` to install.

### Manual

```bash
git clone https://github.com/elmomk/tmux_cc_attention ~/tmux_cc_attention
```

Add to `~/.tmux.conf`:

```tmux
set -g @claude-theme 'catppuccin-mocha'
run-shell '~/tmux_cc_attention/claude-attention.tmux'
```

### Claude Code hooks

```bash
~/.tmux/plugins/tmux_cc_attention/scripts/setup.sh --apply
```

Verify with:

```bash
~/.tmux/plugins/tmux_cc_attention/scripts/setup.sh --check
```

## How it works

```
Claude Code PreToolUse hook   → active.sh  → * green  (working)
Claude Code Notification hook → notify.sh  → ! red    (needs input)
Claude Code Stop hook         → stopped.sh → - blue   (stopped)
tmux status-right             → status.sh  → !3 *2 -1 (cross-session counts)
```

State is stored in tmux window options (`@claude-active`, `@claude-attention`, `@claude-stopped`). No temp files. Each state freely transitions to any other — the last hook to fire wins.

## Themes

Set `@claude-theme` before the plugin loads:

```tmux
set -g @claude-theme 'kanagawa-dragon'
# Options: kanagawa-dragon, catppuccin-mocha, tokyonight, dracula
```

Each theme sets matching colors for all indicators. The selected window tab uses the theme's purple/accent; green is reserved for active Claude sessions.

## Session Dashboard & Quick Switcher

An optional popup that shows all Claude windows with their state and lets you jump to any one. Requires [fzf](https://github.com/junegunn/fzf).

Add to `~/.tmux.conf` (before the plugin loads):

```tmux
set -g @claude-popup-key 'G'   # prefix + G opens the popup
```

If `@claude-popup-key` is not set, no keybinding is registered.

The popup shows each Claude window with a state icon (`!` attention, `*` working, `-` stopped), a live preview of the terminal content, and switches to the selected window on enter.

## Configuration

| Option | Default | Purpose |
|--------|---------|---------|
| `@claude-theme` | `kanagawa-dragon` | Theme to load |
| `@claude-popup-key` | *(unset)* | Key to open session dashboard popup (opt-in) |
| `@claude-active-color` | theme green | Color for active Claude windows |
| `@claude-attention-color` | theme red | Color for attention windows |
| `@claude-stopped-color` | theme blue | Color for stopped windows |
| `@claude-attention-bell` | `off` | Also trigger tmux bell on notification |
| `@claude-window-bg` | theme surface | Background color for window labels |

## Known limitations

- Multiple Claude instances in the same tmux window share one state indicator (window-level, not pane-level)
- Requires tmux >= 3.0 for indexed hooks

## License

AGPL-3.0
