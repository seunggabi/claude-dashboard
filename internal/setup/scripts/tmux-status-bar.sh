#!/usr/bin/env bash
# Display claude-dashboard version and update notification in tmux status bar

VERSION_FILE="$HOME/.cache/claude-dashboard/version"
CURRENT_VERSION_FILE="$HOME/.cache/claude-dashboard/current-version"
LAST_CHECK_FILE="$HOME/.cache/claude-dashboard/last-check"

# Create cache directory if it doesn't exist
mkdir -p "$HOME/.cache/claude-dashboard"

# Check for updates every hour
CURRENT_TIME=$(date +%s)
LAST_CHECK=0
if [ -f "$LAST_CHECK_FILE" ]; then
    LAST_CHECK=$(cat "$LAST_CHECK_FILE")
fi

# 3600 seconds = 1 hour
if [ $((CURRENT_TIME - LAST_CHECK)) -gt 3600 ]; then
    # Get current installed version
    if command -v claude-dashboard &> /dev/null; then
        CURRENT=$(claude-dashboard --version 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' || echo "v0.0.0")
        echo "$CURRENT" > "$CURRENT_VERSION_FILE"
    fi

    # Get latest version from GitHub
    LATEST=$(curl -s https://api.github.com/repos/seunggabi/claude-dashboard/releases/latest | grep -oE '"tag_name": "v[0-9]+\.[0-9]+\.[0-9]+"' | cut -d'"' -f4 || echo "")

    if [ -n "$LATEST" ]; then
        echo "$LATEST" > "$VERSION_FILE"
    fi

    echo "$CURRENT_TIME" > "$LAST_CHECK_FILE"
fi

# Read cached version info
CURRENT="v0.0.0"
LATEST=""
if [ -f "$CURRENT_VERSION_FILE" ]; then
    CURRENT=$(cat "$CURRENT_VERSION_FILE")
fi
if [ -f "$VERSION_FILE" ]; then
    LATEST=$(cat "$VERSION_FILE")
fi

# Display version with update notification
if [ -n "$LATEST" ] && [ "$CURRENT" != "$LATEST" ]; then
    echo "📦 $CURRENT → $LATEST"
else
    echo "✓ $CURRENT"
fi
