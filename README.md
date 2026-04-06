# tmux_cc_attention

A tmux plugin that provides visual indicators for Claude Code session state. Windows show `*` green when Claude is working, `!` red when it needs attention, and `-` blue when it stops — with cross-session awareness, desktop notifications, and bundled color themes.

Powered by a Go state daemon (`claude-state`) that manages all plugin state centrally. Zero shell dependencies at runtime.

Requires **tmux >= 3.0** and **Go >= 1.23** (for building).

## Features

- **Active indicator**: `* green` — Claude Code is working
- **Attention indicator**: `! red` — Claude Code needs your input
- **Stopped indicator**: `- blue` — Claude Code has stopped
- **Persistent state**: Colors stay until the actual state changes
- **Current window**: State visible even on the selected window tab
- **Cross-session**: Status bar shows `!3 *2 -1` counts from other sessions
- **Desktop notifications**: Linux (`notify-send`) and Windows (PowerShell toast) with click-to-focus
- **Colorblind-friendly**: Text prefixes (`*`, `!`, `-`) alongside colors
- **Session dashboard**: `prefix + G` popup to see all Claude windows and jump to one (opt-in, requires fzf)
- **Done notification**: Inline status-right indicator when Claude finishes working (opt-in)
- **Stale cleanup**: Daemon detects when Claude exits and removes orphaned markers
- **Themes**: Kanagawa Dragon, Catppuccin Mocha, Tokyo Night, Dracula

## Installation

### Via TPM

Add to `~/.tmux.conf`:

```tmux
# -- Claude Code Attention Plugin --
set -g @claude-theme 'catppuccin-mocha'  # or kanagawa-dragon, tokyonight, dracula
set -g @claude-popup-key 'G'             # prefix + G opens session dashboard (optional, requires fzf)
set -g @claude-done-popup 'on'           # show inline notification when Claude finishes (optional)
set -g @plugin 'elmomk/tmux_cc_attention'

run '~/.tmux/plugins/tpm/tpm'
```

Then press `prefix + I` to install. The binary is downloaded automatically from GitHub Releases. If no prebuilt binary is available for your platform, it falls back to building from source (requires `go` in PATH).

### Manual

```bash
git clone https://github.com/elmomk/tmux_cc_attention ~/tmux_cc_attention
cd ~/tmux_cc_attention && make build   # or let tmux download it on first load
```

Add to `~/.tmux.conf`:

```tmux
set -g @claude-theme 'catppuccin-mocha'
set -g @claude-popup-key 'G'       # optional
set -g @claude-done-popup 'on'     # optional
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
Claude Code hooks     →  claude-state <verb>  →  daemon (unix socket)  →  tmux
                                                                        →  notify-send
```

A single Go binary (`claude-state`) operates in two modes:

| Mode | What it does |
|------|-------------|
| `claude-state serve` | Long-lived daemon managing all state, counts, cleanup, and notifications |
| `claude-state active` | Client: reads `$TMUX_PANE`, sends message to daemon, exits in ~1ms |
| `claude-state attention` | Client: marks window as needs-input |
| `claude-state stopped` | Client: marks window as idle/done |
| `claude-state clear` | Client: clears attention/stopped on focused window |
| `claude-state notify` | Client: sends desktop notification with click-to-focus |

The daemon auto-starts when the first client runs. It auto-exits after 10 minutes of inactivity.

State is tracked in-memory by the daemon and mirrored to tmux window options (`@claude-active`, `@claude-attention`, `@claude-stopped`) for display.

## Configuration

| Option | Default | Purpose |
|--------|---------|---------|
| `@claude-theme` | `kanagawa-dragon` | Theme to load |
| `@claude-popup-key` | *(unset)* | Key to open session dashboard popup (opt-in) |
| `@claude-done-popup` | `off` | Show a message when Claude finishes (green->blue) |
| `@claude-active-color` | theme green | Color for active Claude windows |
| `@claude-attention-color` | theme red | Color for attention windows |
| `@claude-stopped-color` | theme blue | Color for stopped windows |
| `@claude-attention-bell` | `off` | Also trigger tmux bell on notification |
| `@claude-stopped-timeout` | *(unset)* | Auto-clear stopped marker after N seconds |
| `@claude-notify-platform` | `auto` | Notification backend: `auto`, `linux`, or `windows` |
| `@claude-auto-name` | `off` | Auto-name new windows after cwd basename |
| `@claude-nav-keys` | `off` | Prefix-less navigation (M-1..9, M-h/l, M-H/L) |
| `@claude-window-bg` | theme surface | Background color for window labels |

## Themes

Set `@claude-theme` before the plugin loads:

```tmux
set -g @claude-theme 'kanagawa-dragon'
# Options: kanagawa-dragon, catppuccin-mocha, tokyonight, dracula
```

Each theme sets matching colors for all indicators. The selected window tab uses the theme's purple/accent; green is reserved for active Claude sessions.

## Session Dashboard & Quick Switcher

An optional popup that shows all Claude windows with their state and lets you jump to any one. Requires [fzf](https://github.com/junegunn/fzf).

```tmux
set -g @claude-popup-key 'G'   # prefix + G opens the popup
```

The popup shows each Claude window with a state icon (`!` attention, `*` working, `-` stopped), a live preview of the terminal content, and switches to the selected window on enter.

## Known limitations

- Multiple Claude instances in the same tmux window share one state indicator (window-level, not pane-level)
- Requires tmux >= 3.0 for indexed hooks
- Go toolchain needed at build time (not at runtime)

## License

AGPL-3.0
