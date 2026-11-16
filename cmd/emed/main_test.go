package main

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestParseCLIArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    cliOptions
		wantErr bool
	}{
		{
			name: "no args",
			args: nil,
			want: cliOptions{},
		},
		{
			name: "claude short",
			args: []string{"-c"},
			want: cliOptions{launchClaude: true},
		},
		{
			name: "claude long with path",
			args: []string{"--claude", "."},
			want: cliOptions{launchClaude: true, pathArg: "."},
		},
		{
			name: "help",
			args: []string{"--help"},
			want: cliOptions{showHelp: true},
		},
		{
			name: "version",
			args: []string{"-v"},
			want: cliOptions{showVersion: true},
		},
		{
			name: "path only",
			args: []string{"/tmp"},
			want: cliOptions{pathArg: "/tmp"},
		},
		{
			name: "double dash path",
			args: []string{"--", "-file.txt"},
			want: cliOptions{pathArg: "-file.txt"},
		},
		{
			name:    "unknown option",
			args:    []string{"--nope"},
			wantErr: true,
		},
		{
			name:    "multiple paths",
			args:    []string{"a", "b"},
			wantErr: true,
		},
		{
			name:    "multiple after double dash",
			args:    []string{"--", "a", "b"},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseCLIArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("unexpected result: got %+v want %+v", got, tc.want)
			}
		})
	}
}

func TestParseBoolEnv(t *testing.T) {
	t.Setenv("EMED_TEST_BOOL", "")
	if value, ok := parseBoolEnv("EMED_TEST_BOOL"); ok || value {
		t.Fatalf("expected empty value to be unset, got value=%t ok=%t", value, ok)
	}

	t.Setenv("EMED_TEST_BOOL", "true")
	if value, ok := parseBoolEnv("EMED_TEST_BOOL"); !ok || !value {
		t.Fatalf("expected true to parse as true, got value=%t ok=%t", value, ok)
	}

	t.Setenv("EMED_TEST_BOOL", "off")
	if value, ok := parseBoolEnv("EMED_TEST_BOOL"); !ok || value {
		t.Fatalf("expected off to parse as false, got value=%t ok=%t", value, ok)
	}

	t.Setenv("EMED_TEST_BOOL", "maybe")
	if value, ok := parseBoolEnv("EMED_TEST_BOOL"); ok || value {
		t.Fatalf("expected invalid value to be unset, got value=%t ok=%t", value, ok)
	}
}

func TestShouldEnableTerminalPaste(t *testing.T) {
	tests := []struct {
		name         string
		enablePaste  string
		disablePaste string
		want         bool
	}{
		{
			name: "default",
			want: true,
		},
		{
			name:        "enable override false",
			enablePaste: "0",
			want:        false,
		},
		{
			name:        "enable override true",
			enablePaste: "1",
			want:        true,
		},
		{
			name:         "disable override true",
			disablePaste: "1",
			want:         false,
		},
		{
			name:         "disable override false",
			disablePaste: "0",
			want:         true,
		},
		{
			name:        "enable override true with extra env values",
			enablePaste: "1",
			want:        true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("EMED_ENABLE_PASTE", tc.enablePaste)
			t.Setenv("EMED_DISABLE_PASTE", tc.disablePaste)
			t.Setenv("SSH_CONNECTION", "127.0.0.1 123 127.0.0.1 22")
			t.Setenv("SSH_TTY", "/dev/ttys000")
			t.Setenv("TERM_PROGRAM", "iTerm.app")

			got := shouldEnableTerminalPaste()
			if got != tc.want {
				t.Fatalf("unexpected result: got %t want %t", got, tc.want)
			}
		})
	}
}

func TestRunApp(t *testing.T) {
	tests := []struct {
		name      string
		run       func() error
		wantCode  int
		wantOut   string
		wantStack bool
		wantStop  bool
	}{
		{
			name:     "clean exit",
			run:      func() error { return nil },
			wantCode: 0,
			wantStop: true,
		},
		{
			name:     "run error",
			run:      func() error { return errors.New("terminal not available") },
			wantCode: 1,
			wantOut:  "Error: terminal not available\n",
		},
		{
			name:      "panic",
			run:       func() error { panic("boom") },
			wantCode:  1,
			wantStack: true,
			wantStop:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer
			var logged []string
			cleanups := 0
			var stopApp bool

			code := runApp(tc.run, func(stop bool) { cleanups++; stopApp = stop }, &stderr, func(format string, args ...interface{}) {
				logged = append(logged, fmt.Sprintf(format, args...))
			})

			if code != tc.wantCode {
				t.Errorf("exit code: got %d want %d", code, tc.wantCode)
			}
			if cleanups != 1 {
				t.Errorf("cleanup calls: got %d want 1", cleanups)
			}
			if stopApp != tc.wantStop {
				t.Errorf("stopApp: got %t want %t", stopApp, tc.wantStop)
			}
			out := stderr.String()
			if tc.wantStack {
				if !strings.HasPrefix(out, "Application error: boom\nStack trace:\n") || !strings.Contains(out, "goroutine") {
					t.Errorf("stderr should hold the panic and its stack trace, got %q", out)
				}
				if len(logged) != 1 || !strings.Contains(logged[0], "goroutine") {
					t.Errorf("stack trace should be logged, got %q", logged)
				}
				return
			}
			if out != tc.wantOut {
				t.Errorf("stderr: got %q want %q", out, tc.wantOut)
			}
			if strings.Contains(strings.Join(logged, "\n"), "goroutine") {
				t.Errorf("no stack trace expected in the log, got %q", logged)
			}
			if tc.wantCode != 0 && len(logged) != 1 {
				t.Errorf("error should be logged once, got %q", logged)
			}
		})
	}
}
