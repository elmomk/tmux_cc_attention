Show the full state of the plugin: daemon status, all tracked windows, tmux markers, resource usage, and git state.

Steps:
1. Daemon status: run `bin/claude-state status` and display the JSON (windows, timers, counts, uptime, notifications)
2. Resource usage: `ps -p $(cat /run/user/1000/claude-state/claude-state.pid) -o pid,rss,%cpu,etime --no-headers`
3. tmux markers: `tmux list-windows -a -F '#{session_name}:#{window_index} #{window_name} att=#{@claude-attention} act=#{@claude-active} stop=#{@claude-stopped}'` filtered to windows with any marker set
4. Hooks: `tmux show-hooks -g | grep claude`
5. Installed binary version: `ls -lh ~/.tmux/plugins/tmux_cc_attention/bin/claude-state`
6. State file: `ls -lh /run/user/1000/claude-state/claude-state.json`
7. Git: current branch, latest tag, dirty state
8. Format everything as a clean summary table
