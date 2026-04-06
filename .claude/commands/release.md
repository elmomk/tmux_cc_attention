Build, test, commit, tag, and push a new release. Argument is the version bump type: "patch", "minor", or "major". If no argument, default to "patch".

Steps:
1. Run `go build -ldflags="-s -w" -o bin/claude-state ./cmd/claude-state/` — fail if build errors
2. Run a quick smoke test: start daemon, send active/stopped/status messages, verify responses, kill daemon
3. Determine the next version tag:
   - Read the latest tag with `git tag -l 'v*' --sort=-v:refname | head -1`
   - Parse the semver (major.minor.patch)
   - Bump based on argument: patch (default), minor, or major
   - Example: v3.1.0 + patch = v3.1.1, v3.1.0 + minor = v3.2.0
4. Show the user: what changed (git diff --stat HEAD), proposed version, and ask for confirmation before proceeding
5. Stage all changes: `git add -A`
6. Commit with a descriptive message summarizing the changes
7. Tag with the new version: `git tag <version>`
8. Push: `git push origin main --tags`
9. Wait for GitHub Actions to complete: poll `gh api repos/elmomk/tmux_cc_attention/actions/runs` until the latest run is `completed`
10. Verify release assets exist: `gh api repos/elmomk/tmux_cc_attention/releases/tags/<version>` and list the binary assets
11. Report the release URL
