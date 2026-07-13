package services

import (
	"os/exec"
	"testing"
)

func TestResolvePythonExecutableRejectsOtherCommands(t *testing.T) {
	if got := resolvePythonExecutable("sh"); got != "" {
		t.Fatalf("non-Python executable was accepted: %s", got)
	}
}

func TestResolvePythonExecutableAcceptsInstalledPython(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not installed")
	}
	if got := resolvePythonExecutable("python3"); got == "" {
		t.Fatal("installed Python executable was rejected")
	}
}
