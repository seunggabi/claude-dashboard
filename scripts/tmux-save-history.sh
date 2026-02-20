#!/usr/bin/env bash
# Save tmux pane history to a file

set -euo pipefail

# Default save directory (Desktop if exists, otherwise home)
SAVE_DIR="$HOME/Desktop"
if [ ! -d "$SAVE_DIR" ]; then
    SAVE_DIR="$HOME"
fi

# Generate filename with timestamp (including milliseconds to avoid collisions)
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
SESSION_NAME=$(tmux display-message -p '#S' 2>/dev/null || echo "unknown")
FILENAME="tmux-history_${SESSION_NAME}_${TIMESTAMP}.txt"
FILEPATH="${SAVE_DIR}/${FILENAME}"

# Handle duplicate filenames by appending a counter
COUNTER=1
while [ -f "$FILEPATH" ]; do
    FILEPATH="${SAVE_DIR}/tmux-history_${SESSION_NAME}_${TIMESTAMP}_${COUNTER}.txt"
    ((COUNTER++))
done

# Capture entire pane history (from the beginning with -S -)
# -S - means start from the beginning of the scrollback buffer
# -p prints to stdout
if ! tmux capture-pane -S - -p > "$FILEPATH" 2>/dev/null; then
    tmux display-message "✗ Failed to capture pane history"
    exit 1
fi

# Verify file was created and has content
if [ ! -f "$FILEPATH" ] || [ ! -s "$FILEPATH" ]; then
    tmux display-message "✗ Failed to save history file"
    exit 1
fi

# Get file size for user feedback
FILE_SIZE=$(du -h "$FILEPATH" | cut -f1)

# Display success message with file size
tmux display-message "✓ History saved (${FILE_SIZE}): ${FILEPATH}"

# Optional: Open the file location in Finder (macOS only)
if [[ "$OSTYPE" == "darwin"* ]]; then
    # Just notify, don't auto-open to avoid disruption
    # Uncomment the line below if you want to auto-open Finder
    # open -R "$FILEPATH"
    :
fi
