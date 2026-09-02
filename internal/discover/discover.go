package discover

import (
	"fmt"
	"os"
	"path/filepath"
)

type Runtime string

const (
	Node   Runtime = "node"
	Python Runtime = "python"
	Shell  Runtime = "shell"
)

type Entrypoint struct {
	Runtime Runtime
	File    string
}

// Find applies only the documented, root-level entrypoint conventions.
func Find(dir string) (Entrypoint, error) {
	if exists(filepath.Join(dir, "package.json")) {
		return first(dir, Node, "index.js", "main.js")
	}
	if exists(filepath.Join(dir, "pyproject.toml")) || exists(filepath.Join(dir, "requirements.txt")) {
		return first(dir, Python, "main.py", "__main__.py")
	}

	candidates := []Entrypoint{}
	for _, runtime := range []Runtime{Node, Python, Shell} {
		files := map[Runtime][]string{
			Node:   {"index.js", "main.js"},
			Python: {"main.py", "__main__.py"},
			Shell:  {"main.sh", "run.sh"},
		}[runtime]
		for _, name := range files {
			if exists(filepath.Join(dir, name)) {
				candidates = append(candidates, Entrypoint{Runtime: runtime, File: name})
				break
			}
		}
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	if len(candidates) > 1 {
		return Entrypoint{}, fmt.Errorf("ambiguous entrypoint; add package.json, pyproject.toml, or requirements.txt to select a runtime")
	}
	return Entrypoint{}, fmt.Errorf("no entrypoint found; expected index.js/main.js, main.py/__main__.py, or main.sh/run.sh")
}

func first(dir string, runtime Runtime, names ...string) (Entrypoint, error) {
	for _, name := range names {
		if exists(filepath.Join(dir, name)) {
			return Entrypoint{Runtime: runtime, File: name}, nil
		}
	}
	return Entrypoint{}, fmt.Errorf("%s runtime detected but no %s entrypoint found", runtime, runtime)
}

func exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
