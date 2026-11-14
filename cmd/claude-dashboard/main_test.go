package main

import "testing"

func TestParseNewArgs(t *testing.T) {
	tests := []struct {
		desc           string
		args           []string
		name, path, ca string
	}{
		{"no args", nil, "", "", ""},
		{"name only", []string{"proj"}, "proj", "", ""},
		{"name and path", []string{"proj", "--path", "/tmp/x"}, "proj", "/tmp/x", ""},
		{"flags without name are forwarded", []string{"-r"}, "", "", "-r"},
		{"args value and unknown flags are joined in order", []string{"proj", "-c", "--args", "--model opus"}, "proj", "", "-c --model opus"},
		{"path without value is ignored", []string{"proj", "--path"}, "proj", "", ""},
	}
	for _, tt := range tests {
		name, path, ca := parseNewArgs(tt.args)
		if name != tt.name || path != tt.path || ca != tt.ca {
			t.Errorf("%s: parseNewArgs(%q) = (%q, %q, %q), want (%q, %q, %q)",
				tt.desc, tt.args, name, path, ca, tt.name, tt.path, tt.ca)
		}
	}
}

func TestDefaultSessionName(t *testing.T) {
	tests := []struct{ path, home, want string }{
		{"/home/me/project/foo", "/home/me", "project-foo"},
		{"/home/me/code/my.app", "/home/me", "code-my-app"},
		{"/home/me", "/home/me", "me"},
		{"/home/meow/x", "/home/me", "-home-meow-x"},
		{"/tmp/x y", "/home/me", "-tmp-x-y"},
		{"/tmp/x", "", "-tmp-x"},
	}
	for _, tt := range tests {
		if got := defaultSessionName(tt.path, tt.home); got != tt.want {
			t.Errorf("defaultSessionName(%q, %q) = %q, want %q", tt.path, tt.home, got, tt.want)
		}
	}
}
