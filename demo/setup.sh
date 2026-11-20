# Prepares a throwaway environment for recording demo/demo.gif.
# Sourced by demo.tape from the repository root; not meant to be run directly.

# demo.tape writes its screenshots here.
mkdir -p demo/shots

# A temporary HOME and tmux socket keep the recording away from real config and sessions.
export HOME="$(mktemp -d)"
export TMUX_TMPDIR="$HOME"
export PATH="$PWD/demo/stub:$PWD/bin:$PATH"

mkdir -p "$HOME/code/api-server" "$HOME/code/web-app" "$HOME/code/infra"
git init -q -b feat/rate-limit "$HOME/code/api-server"
git init -q -b feat/settings-forms "$HOME/code/web-app"
git init -q -b main "$HOME/code/infra"
tmux new-session -d -s cd-api-server -c "$HOME/code/api-server" "claude working"
tmux new-session -d -s cd-web-app -c "$HOME/code/web-app" "claude waiting"
tmux new-session -d -s cd-infra -c "$HOME/code/infra" "claude idle"

claude-dashboard setup >/dev/null 2>&1

# Conversation logs for the overview view (o). Claude Code names the log
# directory after the working directory with every non-alphanumeric
# character replaced by "-".
demo_log() {
  local dir="$HOME/.claude/projects/$(printf '%s' "$1" | sed 's/[^a-zA-Z0-9]/-/g')"
  mkdir -p "$dir"
  printf '%s\n' "${@:2}" > "$dir/demo.jsonl"
}
demo_log "$HOME/code/api-server" \
  '{"type":"user","message":{"role":"user","content":"Add rate limiting to the login endpoint"}}' \
  '{"type":"ai-title","aiTitle":"Rate limit the login endpoint"}' \
  '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"TodoWrite","input":{"todos":[{"content":"Read the auth middleware","status":"completed"},{"content":"Add a token bucket per IP","status":"in_progress"},{"content":"Write tests for 429 responses","status":"pending"}]}}]}}'
demo_log "$HOME/code/web-app" \
  '{"type":"user","message":{"role":"user","content":"Migrate the settings page to the new form library"}}' \
  '{"type":"ai-title","aiTitle":"Migrate settings page forms"}' \
  '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"TodoWrite","input":{"todos":[{"content":"Replace the profile form","status":"completed"},{"content":"Replace the billing form","status":"completed"},{"content":"Remove the old form library","status":"pending"}]}}]}}'

cd "$HOME/code"
PS1='$ '
clear
