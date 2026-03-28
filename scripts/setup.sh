#!/usr/bin/env bash
# Prints or applies the Claude Code hooks configuration needed for this plugin.

set -euo pipefail

CURRENT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ACTIVE_SCRIPT="$CURRENT_DIR/active.sh"
NOTIFY_SCRIPT="$CURRENT_DIR/notify.sh"
STOPPED_SCRIPT="$CURRENT_DIR/stopped.sh"
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
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "$NOTIFY_SCRIPT"
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
    echo "Add the following to $SETTINGS_FILE:"
    echo ""
    hook_json
    echo ""
    echo "Or run with --apply to merge automatically (requires jq)."
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

    # Check if all hooks are already installed
    local active_installed notify_installed stop_installed
    active_installed=$(jq -e --arg cmd "$ACTIVE_SCRIPT" '
        .hooks.PreToolUse // [] | map(.hooks // [] | map(.command)) | flatten | any(. == $cmd)
    ' "$SETTINGS_FILE" 2>/dev/null && echo "yes" || echo "no")

    notify_installed=$(jq -e --arg cmd "$NOTIFY_SCRIPT" '
        .hooks.Notification // [] | map(.hooks // [] | map(.command)) | flatten | any(. == $cmd)
    ' "$SETTINGS_FILE" 2>/dev/null && echo "yes" || echo "no")

    stop_installed=$(jq -e --arg cmd "$STOPPED_SCRIPT" '
        .hooks.Stop // [] | map(.hooks // [] | map(.command)) | flatten | any(. == $cmd)
    ' "$SETTINGS_FILE" 2>/dev/null && echo "yes" || echo "no")

    if [ "$active_installed" = "yes" ] && [ "$notify_installed" = "yes" ] && [ "$stop_installed" = "yes" ]; then
        echo "All hooks already installed (PreToolUse, Notification, Stop)."
        return
    fi

    # Back up existing settings
    cp "$SETTINGS_FILE" "${SETTINGS_FILE}.bak"

    # Merge: deep-merge our hook into existing settings
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
    ' "$SETTINGS_FILE" <(hook_json) > "${SETTINGS_FILE}.tmp"
    mv "${SETTINGS_FILE}.tmp" "$SETTINGS_FILE"

    echo "Updated $SETTINGS_FILE (backup at ${SETTINGS_FILE}.bak)."
    echo "Hooks point to:"
    echo "  PreToolUse:   $ACTIVE_SCRIPT"
    echo "  Notification: $NOTIFY_SCRIPT"
    echo "  Stop:         $STOPPED_SCRIPT"
}

case "${1:-}" in
    --apply)
        apply_hook
        ;;
    --help|-h)
        print_usage
        ;;
    *)
        print_usage
        ;;
esac
