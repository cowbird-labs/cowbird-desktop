package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	old := version
	version = "9.9.9-test"
	defer func() { version = old }()

	var out bytes.Buffer
	root := newRootCmd(func() { t.Fatal("runGUI must not be called for a subcommand") })
	root.SetOut(&out)
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("version command returned error: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "9.9.9-test") {
		t.Errorf("version output %q does not contain the version string", got)
	}
}

func TestUnknownCommand(t *testing.T) {
	called := false
	root := newRootCmd(func() { called = true })
	root.SetArgs([]string{"frobnicate"})
	// Silence Cobra's error/usage output during the test.
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})

	if err := root.Execute(); err == nil {
		t.Fatal("expected an error for an unknown command, got nil")
	}
	if called {
		t.Error("runGUI must not be invoked for an unknown command")
	}
}

func TestBareInvocationRunsGUI(t *testing.T) {
	count := 0
	root := newRootCmd(func() { count++ })
	root.SetArgs([]string{})

	if err := root.Execute(); err != nil {
		t.Fatalf("bare invocation returned error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected runGUI to be called once, got %d", count)
	}
}
