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
	Shell  Runtime = "bash"
)

type Entrypoint struct {
	Runtime Runtime
	File    string
}

// Find applies the single entrypoint convention encoded by the repository suffix.
func Find(dir, repo string) (Entrypoint, error) {
	runtime, err := FromRepository(repo)
	if err != nil {
		return Entrypoint{}, err
	}
	switch filepath.Ext(repo) {
	case ".js":
		return findOne(dir, runtime, "index.js", "index.mjs")
	case ".py":
		return find(dir, runtime, "main.py")
	case ".sh":
		return find(dir, runtime, "main.sh")
	default:
		return Entrypoint{}, fmt.Errorf("unsupported repository suffix")
	}
}

func FromRepository(repo string) (Runtime, error) {
	switch filepath.Ext(repo) {
	case ".js":
		return Node, nil
	case ".py":
		return Python, nil
	case ".sh":
		return Shell, nil
	default:
		return "", fmt.Errorf("repository name must end in .sh, .js, or .py")
	}
}

func findOne(dir string, runtime Runtime, names ...string) (Entrypoint, error) {
	var found string
	for _, name := range names {
		if !exists(filepath.Join(dir, name)) {
			continue
		}
		if found != "" {
			return Entrypoint{}, fmt.Errorf("%s snippet has multiple entrypoints: %s and %s", runtime, found, name)
		}
		found = name
	}
	if found == "" {
		return Entrypoint{}, fmt.Errorf("%s snippet requires index.js or index.mjs", runtime)
	}
	return Entrypoint{Runtime: runtime, File: found}, nil
}

func find(dir string, runtime Runtime, name string) (Entrypoint, error) {
	if exists(filepath.Join(dir, name)) {
		return Entrypoint{Runtime: runtime, File: name}, nil
	}
	return Entrypoint{}, fmt.Errorf("%s snippet requires %s", runtime, name)
}

func exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
