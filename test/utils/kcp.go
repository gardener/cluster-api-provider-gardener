package utils

import (
	"fmt"
	"os/exec"
	"strings"
)

// EnsureAndSwitchWorkspace ensures that the specified workspace exists in kcp, else creates it, and switches to it.
func EnsureAndSwitchWorkspace(workspacePath ...string) error {
	expectedWorkspacePath := "root:" + strings.Join(workspacePath, ":")
	currentWorkspace, err := Run(exec.Command("kubectl", "kcp", "ws", "."))
	fmt.Printf("Current workspace: %q, expected workspace: %q\n", currentWorkspace, expectedWorkspacePath)
	if strings.Contains(currentWorkspace, fmt.Sprintf("'%s'", expectedWorkspacePath)) {
		return err
	}
	_, err = Run(exec.Command("kubectl", "kcp", "ws", ":root"))
	if err != nil {
		return err
	}
	for _, path := range workspacePath {
		_, err = Run(exec.Command("kubectl", "kcp", "ws", path))
		if err != nil {
			_, err = Run(exec.Command("kubectl", "create workspace", path, "--enter"))
		}
		if err != nil {
			return err
		}
	}
	return nil
}
