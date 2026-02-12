#!/usr/bin/env bash
# Toggle tmux mouse mode and display status

if tmux show-option -gv mouse | grep -q on; then
    tmux set-option -g mouse off
    tmux display-message "Mouse: OFF"
else
    tmux set-option -g mouse on
    tmux display-message "Mouse: ON"
fi

# Refresh client to update status bar immediately
tmux refresh-client -S
