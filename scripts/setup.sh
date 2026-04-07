#!/usr/bin/env bash
# Prints or applies the Claude Code hooks configuration needed for this plugin.

set -euo pipefail

CURRENT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLAUDE_STATE="$CURRENT_DIR/../bin/claude-state"
ACTIVE_SCRIPT="$CLAUDE_STATE active"
NOTIFY_SCRIPT="$CLAUDE_STATE notify"
STOPPED_SCRIPT="$CLAUDE_STATE stopped"
SETTINGS_FILE="$HOME/.claude/settings.json"

hook_json() {
    cat <<EOF
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "$ACTIVE_SCRIPT"
          }
        ]
      }
    ],
    "Notification": [
      {
        "matcher": "permission_prompt|elicitation_dialog",
        "hooks": [
          {
            "type": "command",
            "command": "$NOTIFY_SCRIPT"
          }
        ]
      },
      {
        "matcher": "idle_prompt",
        "hooks": [
          {
            "type": "command",
            "command": "$STOPPED_SCRIPT"
          }
        ]
      }
    ],
    "Stop": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "$STOPPED_SCRIPT"
          }
        ]
      }
    ]
  }
}
EOF
}

print_usage() {
    echo "Claude Code Attention Plugin - Hook Setup"
    echo ""
    echo "Commands:"
    echo "  --apply   Install hooks into $SETTINGS_FILE"
    echo "  --check   Verify installation"
    echo "  --help    Show hook JSON"
    echo ""
    echo "Hook JSON:"
    echo ""
    hook_json
}

check_install() {
    local ok=true

    echo "=== Binary ==="
    if [ -x "$CLAUDE_STATE" ]; then
        echo "  OK: $CLAUDE_STATE"
    else
        echo "FAIL: claude-state binary not found at $CLAUDE_STATE"
        echo "      Run 'make build' or reload tmux to download it"
        ok=false
    fi

    echo ""
    echo "=== Claude Code Hooks ==="
    if [ ! -f "$SETTINGS_FILE" ]; then
        echo "FAIL: $SETTINGS_FILE not found"
        ok=false
    else
        for hook_type in PreToolUse Notification Stop; do
            local count
            count=$(jq -r ".hooks.${hook_type} // [] | length" "$SETTINGS_FILE" 2>/dev/null || echo 0)
            local expected=1
            [ "$hook_type" = "Notification" ] && expected=2
            if [ "$count" -eq 0 ]; then
                echo "FAIL: No $hook_type hooks"
                ok=false
            elif [ "$count" -ne "$expected" ]; then
                echo "WARN: $hook_type has $count entries (expected $expected) — run --apply to fix"
                ok=false
            else
                echo "  OK: $hook_type"
            fi
        done
    fi

    echo ""
    echo "=== tmux Hooks ==="
    local theme
    theme=$(tmux show-option -gqv @claude-theme 2>/dev/null || echo "not set")
    echo "  Theme: $theme"

    local plugin_path
    plugin_path=$(tmux show-option -gqv @claude-attention-plugin-path 2>/dev/null || echo "not set")
    echo "  Plugin path: $plugin_path"

    # Verify auto-clear hooks are installed
    for hook_name in session-window-changed client-session-changed; do
        if tmux show-hooks -g 2>/dev/null | grep -q "${hook_name}\[100\]"; then
            echo "  OK: ${hook_name}[100] auto-clear hook"
        else
            echo "WARN: ${hook_name}[100] auto-clear hook missing — reload tmux config"
            ok=false
        fi
    done

    echo ""
    echo "=== Notifications ==="
    local notify_platform
    notify_platform=$(tmux show-option -gqv @claude-notify-platform 2>/dev/null || echo "auto")
    echo "  Platform: $notify_platform"
    if [ -n "$WSL_DISTRO_NAME" ] || [ -n "$WT_SESSION" ]; then
        if [ "$notify_platform" != "windows" ]; then
            echo "WARN: WSL detected but @claude-notify-platform is not 'windows'"
            echo "      notify-send won't work without a D-Bus daemon"
            echo "      Run: tmux set -g @claude-notify-platform 'windows'"
            ok=false
        fi
        local notify_appid
        notify_appid=$(tmux show-option -gqv @claude-notify-appid 2>/dev/null)
        if [ -n "$notify_appid" ]; then
            echo "  AppID:    $notify_appid"
        else
            echo "  AppID:    default (Windows Terminal)"
        fi
    fi

    echo ""
    echo "=== Optional Features ==="
    local auto_name
    auto_name=$(tmux show-option -gqv @claude-auto-name 2>/dev/null || echo "off")
    echo "  Auto-name windows: $auto_name  (set @claude-auto-name 'on' to enable)"
    local nav_keys
    nav_keys=$(tmux show-option -gqv @claude-nav-keys 2>/dev/null || echo "off")
    echo "  Prefix-less nav:   $nav_keys  (set @claude-nav-keys 'on' to enable)"

    echo ""
    echo "=== Colors ==="
    echo "  Active:    $(tmux show-option -gqv @claude-active-color 2>/dev/null || echo 'default')"
    echo "  Attention: $(tmux show-option -gqv @claude-attention-color 2>/dev/null || echo 'default')"
    echo "  Stopped:   $(tmux show-option -gqv @claude-stopped-color 2>/dev/null || echo 'default')"

    echo ""
    echo "=== Legend ==="
    echo "  * green  = Claude is working"
    echo "  ! red    = Claude needs your input"
    echo "  - blue   = Claude is idle or has stopped"

    if [ "$ok" = true ]; then
        echo ""
        echo "All checks passed."
    fi
}

apply_hook() {
    if ! command -v jq &>/dev/null; then
        echo "Error: jq is required for --apply. Install it or add the config manually." >&2
        exit 1
    fi

    mkdir -p "$(dirname "$SETTINGS_FILE")"

    if [ ! -f "$SETTINGS_FILE" ]; then
        hook_json > "$SETTINGS_FILE"
        echo "Created $SETTINGS_FILE with PreToolUse, Notification, and Stop hooks."
        return
    fi

    # Back up existing settings
    cp "$SETTINGS_FILE" "${SETTINGS_FILE}.bak"

    # Remove existing tmux_cc_attention hook entries (scoped to our plugin paths),
    # then add fresh ones. This prevents duplicates from repeated --apply runs or path changes.
    jq '
        def is_plugin_hook:
            (.hooks // []) | any(.command | (
                contains("tmux_cc_attention") or contains("tmux_claude_code_plugin")
            ));
        def remove_plugin_hooks:
            if . == null then []
            else [.[] | select(is_plugin_hook | not)]
            end;
        .hooks.PreToolUse = (.hooks.PreToolUse | remove_plugin_hooks) |
        .hooks.Notification = (.hooks.Notification | remove_plugin_hooks) |
        .hooks.Stop = (.hooks.Stop | remove_plugin_hooks)
    ' "$SETTINGS_FILE" > "${SETTINGS_FILE}.tmp"

    # Now merge in our fresh hooks
    jq -s '
        def deep_merge(a; b):
            a as $a | b as $b |
            if ($a | type) == "object" and ($b | type) == "object" then
                ($a | keys_unsorted) + ($b | keys_unsorted) | unique |
                map(. as $k | {($k): deep_merge($a[$k]; $b[$k])}) |
                add // {}
            elif ($a | type) == "array" and ($b | type) == "array" then
                $a + $b
            elif $b != null then $b
            else $a
            end;
        deep_merge(.[0]; .[1])
    ' "${SETTINGS_FILE}.tmp" <(hook_json) > "${SETTINGS_FILE}.tmp2"
    mv "${SETTINGS_FILE}.tmp2" "$SETTINGS_FILE"
    rm -f "${SETTINGS_FILE}.tmp"

    echo "Updated $SETTINGS_FILE (backup at ${SETTINGS_FILE}.bak)."
    echo "Hooks point to:"
    echo "  PreToolUse:   $ACTIVE_SCRIPT"
    echo "  Notification: $NOTIFY_SCRIPT (attention), $STOPPED_SCRIPT (idle)"
    echo "  Stop:         $STOPPED_SCRIPT"

    # WSL: auto-configure Windows notifications + click-to-focus shortcut
    if [ -n "$WSL_DISTRO_NAME" ] || [ -n "$WT_SESSION" ]; then
        local current_platform
        current_platform=$(tmux show-option -gqv @claude-notify-platform 2>/dev/null)
        if [ "$current_platform" != "windows" ]; then
            tmux set-option -g @claude-notify-platform 'windows'
            echo ""
            echo "WSL detected — set @claude-notify-platform 'windows'"
        fi

        # Register click-to-focus shortcut with custom AUMID
        local aumid="Claude.Code.Notify"
        local focus_cmd
        focus_cmd="$(cd "$CURRENT_DIR/.." && pwd)/bin/claude-state focus-click"
        echo ""
        echo "Registering Windows notification shortcut..."
        if powershell.exe -NoProfile -NonInteractive -Command "
\$lnkPath = \"\$env:APPDATA\\Microsoft\\Windows\\Start Menu\\Programs\\Claude Code Notify.lnk\"
\$shell = New-Object -ComObject WScript.Shell
\$sc = \$shell.CreateShortcut(\$lnkPath)
\$sc.TargetPath = 'wsl.exe'
\$sc.Arguments = '-- $focus_cmd'
\$sc.Description = 'Claude Code notification click handler'
\$sc.WindowStyle = 7
\$sc.Save()

Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;

[StructLayout(LayoutKind.Sequential, Pack = 4)]
public struct PROPERTYKEY {
    public Guid fmtid;
    public uint pid;
}

[StructLayout(LayoutKind.Explicit)]
public struct PROPVARIANT {
    [FieldOffset(0)] public ushort vt;
    [FieldOffset(8)] public IntPtr pVal;
    public static PROPVARIANT FromString(string s) {
        var pv = new PROPVARIANT();
        pv.vt = 31;
        pv.pVal = Marshal.StringToCoTaskMemUni(s);
        return pv;
    }
}

[ComImport, Guid(\"886D8EEB-8CF2-4446-8D02-CDBA1DBDCF99\"), InterfaceType(ComInterfaceType.InterfaceIsIUnknown)]
public interface IPropertyStore {
    uint GetCount();
    PROPERTYKEY GetAt(uint iProp);
    PROPVARIANT GetValue(ref PROPERTYKEY key);
    void SetValue(ref PROPERTYKEY key, ref PROPVARIANT pv);
    void Commit();
}

public static class PropStore {
    [DllImport(\"shell32.dll\")]
    static extern int SHGetPropertyStoreFromParsingName(
        [MarshalAs(UnmanagedType.LPWStr)] string pszPath,
        IntPtr pbc, uint flags, ref Guid riid,
        [MarshalAs(UnmanagedType.Interface)] out IPropertyStore ppv);

    public static void SetAumid(string path, string aumid) {
        Guid IID = new Guid(\"886D8EEB-8CF2-4446-8D02-CDBA1DBDCF99\");
        IPropertyStore store;
        int hr = SHGetPropertyStoreFromParsingName(path, IntPtr.Zero, 2, ref IID, out store);
        if (hr != 0) throw new System.Runtime.InteropServices.COMException(\"failed\", hr);
        var key = new PROPERTYKEY { fmtid = new Guid(\"9F4C2855-9F79-4B39-A8D0-E1D42DE1D5F3\"), pid = 5 };
        var pv = PROPVARIANT.FromString(aumid);
        store.SetValue(ref key, ref pv);
        store.Commit();
    }
}
'@
[PropStore]::SetAumid(\$lnkPath, '$aumid')

# Register AUMID in notification settings so Windows recognizes it
\$regPath = 'HKCU:\\Software\\Microsoft\\Windows\\CurrentVersion\\Notifications\\Settings\\$aumid'
if (-not (Test-Path \$regPath)) { New-Item -Path \$regPath -Force | Out-Null }

Write-Output 'OK'
" 2>/dev/null | tr -d '\r' | grep -q '^OK$'; then
            tmux set-option -g @claude-notify-appid "$aumid"
            echo "  OK: Shortcut registered with AUMID '$aumid'"
            echo "  Click-to-focus enabled for notifications"
        else
            echo "WARN: Could not register shortcut — falling back to Windows Terminal AppID"
            echo "      Clicking notifications will open a new terminal tab"
        fi
    fi
}

case "${1:-}" in
    --apply)
        apply_hook
        ;;
    --check)
        check_install
        ;;
    --help|-h)
        print_usage
        ;;
    *)
        print_usage
        ;;
esac
