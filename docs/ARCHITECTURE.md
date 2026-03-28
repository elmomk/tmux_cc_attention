# Architecture

## Data Flow

```
                    ┌─────────────┐
                    │ Claude Code  │
                    └──────┬──────┘
                           │
           ┌───────────────┼───────────────┐
           ▼               ▼               ▼
     PreToolUse      Notification        Stop
           │               │               │
           ▼               ▼               ▼
       active.sh      notify.sh       stopped.sh
           │               │               │
           ▼               ▼               ▼
     fg = green       fg = red        fg = dark
     @claude-active   @claude-attention @claude-stopped
           │               │               │
           └───────────────┼───────────────┘
                           │
                    User switches to
                    the window (hook)
                           │
                           ▼
                       clear.sh
                           │
                           ▼
                  Unset format override
                  Unset all markers
```

## State Machine

```
              active.sh                notify.sh
    ┌─────────────────────────┐  ┌──────────────────┐
    │                         ▼  │                   ▼
  NORMAL ─── active.sh ─── ACTIVE ── notify.sh ── ATTENTION
    ▲                         │                      │
    │         stopped.sh      │                      │
    │    ┌────────────────────┘                      │
    │    ▼                                           │
  STOPPED                                            │
    ▲                                                │
    └──────────── clear.sh ──────────────────────────┘
    └──────────── clear.sh (from any state) ─────────┘
```

Priority: **ATTENTION > ACTIVE > STOPPED > NORMAL**

- `notify.sh` always wins — clears `@claude-active` and `@claude-stopped`, sets `@claude-attention`
- `active.sh` skips if `@claude-attention` is set, short-circuits if already active
- `stopped.sh` skips if `@claude-attention` is set, clears `@claude-active`
- `clear.sh` clears all markers and reverts to global format

## Cross-Session Status

`status.sh` is called by tmux's `#()` in `status-right`. Three calls with different color wrappers:

```
#[fg=red]#(status.sh --attention)#[fg=green]#(status.sh --active)#[fg=dark]#(status.sh --stopped)
```

Each call runs `tmux list-windows -a`, filters out windows in the current session, and outputs `session:win` pairs for windows with the relevant marker.

## Hook Registration

Hooks use explicit array indices (`[100]`) instead of `-ga` (global append):

```bash
tmux set-hook -g 'session-window-changed[100]' "run-shell '.../clear.sh'"
tmux set-hook -g 'client-session-changed[100]' "run-shell '.../clear.sh'"
```

Reloading replaces the same index rather than appending duplicates. Old v1 hooks at indices 0-5 are cleaned up on load.

## State Storage

All state lives in **tmux window options** — no temp files:

| Option | Scope | Values | Purpose |
|--------|-------|--------|---------|
| `@claude-active` | per-window | `1` or unset | Claude is actively working |
| `@claude-attention` | per-window | `1` or unset | Claude needs user attention |
| `@claude-stopped` | per-window | `1` or unset | Claude has stopped |
| `window-status-format` | per-window override | format string | Visual color change |

Clearing means unsetting the per-window override, which reverts to the global format.

## Themes

Four bundled themes in `themes/`. Each sets:
- Status bar structure and colors
- Window label formats (purple for selected, theme colors for normal)
- Plugin defaults (`@claude-window-bg`, `@claude-active-color`, `@claude-attention-color`, `@claude-stopped-color`)

The selected window tab uses the theme's purple/accent — green is reserved for active Claude sessions to avoid confusion.

## Design Decisions

### Why tmux options as state store?
- Atomic: tmux handles concurrency
- No cleanup needed: options live with the window
- Queryable: `list-windows -F` can read custom options in bulk

### Why template format strings instead of sed?
With a custom theme we control the format and build it directly via `get_window_format()`. No regex fragility.

### Why indexed hooks instead of `-ga`?
`-ga` appends on every reload, causing duplicates. Indexed hooks (`[100]`) are idempotent.

### Why `PreToolUse` for active state?
It fires whenever Claude uses a tool, reliably indicating active work. The script short-circuits after the first call (`@claude-active` already set) so subsequent calls are cheap.
