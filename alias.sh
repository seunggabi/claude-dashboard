#!/bin/bash
set -e

ALIAS_LINE_BASH="alias cdn='claude-dashboard new'"
ALIAS_LINE_FISH="alias cdn 'claude-dashboard new'"
SHELL_NAME="$(basename "$SHELL")"

case "$SHELL_NAME" in
  zsh)
    RC_FILE="$HOME/.zshrc"
    ALIAS_LINE="$ALIAS_LINE_BASH"
    ;;
  bash)
    if [ -f "$HOME/.bash_profile" ]; then
      RC_FILE="$HOME/.bash_profile"
    else
      RC_FILE="$HOME/.bashrc"
    fi
    ALIAS_LINE="$ALIAS_LINE_BASH"
    ;;
  fish)
    RC_FILE="$HOME/.config/fish/config.fish"
    ALIAS_LINE="$ALIAS_LINE_FISH"
    ;;
  *)
    RC_FILE="$HOME/.${SHELL_NAME}rc"
    ALIAS_LINE="$ALIAS_LINE_BASH"
    ;;
esac

if [ ! -f "$RC_FILE" ]; then
  touch "$RC_FILE"
fi

if grep -qF "alias cdn" "$RC_FILE" 2>/dev/null; then
  echo "cdn alias already exists in $RC_FILE"
else
  echo "" >> "$RC_FILE"
  echo "$ALIAS_LINE" >> "$RC_FILE"
  echo "Added cdn alias to $RC_FILE"
fi

# Apply alias to current shell
eval "$ALIAS_LINE"
echo "cdn is ready to use!"
