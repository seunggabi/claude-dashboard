package main

import (
	"os"
	"testing"
)

// A script run with </dev/null has nobody to answer a prompt, even though
// /dev/null is a character device. Treating it as a terminal made the first-run
// offer "decline" itself permanently.
func TestIsTerminalRejectsDevNull(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if isTerminal(f) {
		t.Fatal("/dev/null must not count as a terminal")
	}
}

func TestIsTerminalRejectsPipesAndFiles(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if isTerminal(r) || isTerminal(w) {
		t.Fatal("a pipe must not count as a terminal")
	}

	file, err := os.CreateTemp(t.TempDir(), "plain")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if isTerminal(file) {
		t.Fatal("a regular file must not count as a terminal")
	}
}
