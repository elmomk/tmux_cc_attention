#!/usr/bin/env bash
# Periodic stale-marker cleanup (runs via #() in status-right).
# Cross-session counts are now push-based via refresh-counts.sh,
# so this script only handles the edge case where Claude exits
# without triggering the Stop hook (e.g., kill -9).
# Outputs nothing — display comes from #{@claude-cross-counts}.

exec "$(dirname "$0")/refresh-counts.sh"
