# Prepares a throwaway environment for recording demo/demo.gif.
# Sourced by demo.tape from the repository root; not meant to be run directly.

# demo.tape writes its screenshots here.
mkdir -p demo/shots

# A temporary HOME and tmux socket keep the recording away from real config and sessions.
export HOME="$(mktemp -d)"
export TMUX_TMPDIR="$HOME"
export PATH="$PWD/demo/stub:$PWD/bin:$PATH"

mkdir -p "$HOME/code/api-server" "$HOME/code/web-app" "$HOME/code/infra"
tmux new-session -d -s cd-api-server -c "$HOME/code/api-server" "claude working"
tmux new-session -d -s cd-web-app -c "$HOME/code/web-app" "claude waiting"
tmux new-session -d -s cd-infra -c "$HOME/code/infra" "claude idle"

claude-dashboard setup >/dev/null 2>&1

cd "$HOME/code"
PS1='$ '
clear
