#!/usr/bin/env bash
# Pull latest plugin and reload tmux config

git -C ~/.tmux/plugins/tmux_cc_attention pull origin main
tmux source-file ~/.tmux.conf
echo "Plugin updated and tmux config reloaded."
