#!/usr/bin/env bash
# Manual clear: removes attention/stopped markers from the current window.
# Routes through the daemon so in-memory state stays consistent.
exec "$(dirname "$0")/../bin/claude-state" clear
