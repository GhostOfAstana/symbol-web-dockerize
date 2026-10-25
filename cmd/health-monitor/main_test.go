package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIUsage(t *testing.T) {
	for _, args := range [][]string{{}, {"--unknown"}, {"a", "b"}, {"--lint", "Dockerfile", "app"}, {"--json", "--lint", "Dockerfile"}} {
		var out, errorsOut bytes.Buffer
		if code := run(args, &out, &errorsOut); code != 2 {
			t.Fatalf("%v: %d", args, code)
		}
		if errorsOut.Len() == 0 {
			t.Fatal("нет объяснения ошибки")
		}
	}
}

func TestCLILint(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "Dockerfile")
	if err := os.WriteFile(path, []byte("FROM golang:1.26-alpine AS build\nFROM alpine:3.23\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, ".dockerignore"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	var out, errorsOut bytes.Buffer
	if code := run([]string{"--lint", path}, &out, &errorsOut); code != 0 {
		t.Fatalf("%d: %s", code, errorsOut.String())
	}
	if !strings.Contains(out.String(), "4/4") {
		t.Fatal(out.String())
	}
	if err := os.Remove(filepath.Join(directory, ".dockerignore")); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"--lint", path}, &out, &errorsOut); code != 1 {
		t.Fatalf("%d", code)
	}
}
