package run

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/snippets-run/runners/internal/discover"
)

func InstallDependencies(dir string, entrypoint discover.Entrypoint) error {
	if entrypoint.Runtime != discover.Node {
		return nil
	}
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err != nil {
		return nil
	}
	pnpm, err := exec.LookPath("pnpm")
	if err != nil {
		return fmt.Errorf("node snippet requires pnpm; install it before running this snippet")
	}
	command := exec.Command(pnpm, "install", "--no-frozen-lockfile")
	command.Dir = dir
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

// Exec replaces the runner process with the snippet runtime.
func Exec(dir string, entrypoint discover.Entrypoint, environment []string) error {
	var runtime string
	switch entrypoint.Runtime {
	case discover.Node:
		runtime = "node"
	case discover.Python:
		runtime = "python3"
	case discover.Shell:
		runtime = "bash"
	default:
		return fmt.Errorf("unsupported runtime %q", entrypoint.Runtime)
	}
	path, err := exec.LookPath(runtime)
	if err != nil {
		return fmt.Errorf("%s is required to run this snippet", runtime)
	}
	if err := os.Chdir(dir); err != nil {
		return err
	}
	return syscall.Exec(path, []string{path, entrypoint.File}, environment)
}
