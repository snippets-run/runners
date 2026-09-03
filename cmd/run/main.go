package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/snippets-run/runners/internal/cache"
	"github.com/snippets-run/runners/internal/discover"
	"github.com/snippets-run/runners/internal/extract"
	"github.com/snippets-run/runners/internal/registry"
	runner "github.com/snippets-run/runners/internal/run"
)

var version = "dev"

const usage = `Usage:
  run <owner>/<name>.<type>@<ref> [--key=value ...]
  run cache status
  run cache clean

Inputs are exposed to snippets as INPUTS_<KEY> environment variables.
Set SNIPPET_CACHE_PATH to override the OS-native cache directory.
Set SNIPPET_REGISTRY_URL to override the registry URL.
`

type invocation struct {
	owner   string
	repo    string
	ref     string
	runtime discover.Runtime
	inputs  map[string]string
}

func main() {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		fail(2, "run supports macOS and Linux only")
	}

	args := os.Args[1:]
	if len(args) == 0 || isHelp(args[0]) {
		fmt.Print(usage)
		if len(args) == 0 {
			os.Exit(2)
		}
		return
	}
	if args[0] == "--version" {
		fmt.Println(version)
		return
	}
	if args[0] == "cache" {
		cacheCommand(args[1:])
		return
	}

	call, err := parseInvocation(args)
	if err != nil {
		fail(2, "%v", err)
	}
	cacheRoot, err := cache.Root(os.Getenv("SNIPPET_CACHE_PATH"))
	if err != nil {
		fail(1, "%v", err)
	}
	client, err := registry.New(os.Getenv("SNIPPET_REGISTRY_URL"), version)
	if err != nil {
		fail(2, "%v", err)
	}

	resolution, err := client.Resolve(context.Background(), call.owner, call.repo, call.ref)
	if err != nil {
		fail(1, "resolve %s/%s@%s: %v", call.owner, call.repo, call.ref, err)
	}
	if (resolution.Owner != "" && resolution.Owner != call.owner) || (resolution.Repo != "" && resolution.Repo != call.repo) {
		fail(1, "registry returned a resolution for a different snippet")
	}
	if !validCommit(resolution.Commit) {
		fail(1, "registry returned an invalid commit")
	}
	if resolution.Type != string(call.runtime) {
		fail(1, "registry returned type %q for %s", resolution.Type, call.repo)
	}

	snippetDir := cache.SnippetDir(cacheRoot, call.owner, call.repo, resolution.Commit)
	if !cache.Ready(snippetDir) {
		if err := prepare(context.Background(), client, call, resolution.Commit, snippetDir); err != nil {
			fail(1, "prepare %s/%s@%s: %v", call.owner, call.repo, call.ref, err)
		}
	}

	entrypoint, err := discover.Find(snippetDir, call.repo)
	if err != nil {
		fail(2, "%v", err)
	}
	if err := runner.InstallDependencies(snippetDir, entrypoint); err != nil {
		fail(1, "install dependencies: %v", err)
	}
	if err := runner.Exec(snippetDir, entrypoint, environment(call.inputs)); err != nil {
		fail(1, "run snippet: %v", err)
	}
}

func prepare(ctx context.Context, client *registry.Client, call invocation, commit, snippetDir string) error {
	if err := cache.EnsureParent(snippetDir); err != nil {
		return err
	}
	archive, err := client.Download(ctx, call.owner, call.repo, commit)
	if err != nil {
		return err
	}
	defer archive.Close()

	temporary := fmt.Sprintf("%s.tmp.%d", snippetDir, os.Getpid())
	if err := os.RemoveAll(temporary); err != nil {
		return err
	}
	if err := os.MkdirAll(temporary, 0o755); err != nil {
		return err
	}
	if err := extract.TarGz(archive, temporary); err != nil {
		_ = os.RemoveAll(temporary)
		return err
	}
	if err := os.Rename(temporary, snippetDir); err != nil {
		_ = os.RemoveAll(temporary)
		if cache.Ready(snippetDir) {
			return nil
		}
		return err
	}
	return nil
}

func cacheCommand(args []string) {
	if len(args) != 1 {
		fail(2, "usage: run cache status|clean")
	}
	root, err := cache.Root(os.Getenv("SNIPPET_CACHE_PATH"))
	if err != nil {
		fail(1, "%v", err)
	}
	switch args[0] {
	case "status":
		size, err := cache.Status(root)
		if err != nil {
			fail(1, "read cache status: %v", err)
		}
		fmt.Println(formatBytes(size))
	case "clean":
		if err := cache.Clean(root); err != nil {
			fail(1, "clean cache: %v", err)
		}
	default:
		fail(2, "unknown cache command %q", args[0])
	}
}

func parseInvocation(args []string) (invocation, error) {
	owner, repo, ref, err := parseIdentifier(args[0])
	if err != nil {
		return invocation{}, err
	}
	runtime, err := discover.FromRepository(repo)
	if err != nil {
		return invocation{}, err
	}
	inputs := make(map[string]string)
	for _, arg := range args[1:] {
		if arg == "--offline" {
			return invocation{}, fmt.Errorf("--offline is not implemented")
		}
		if !strings.HasPrefix(arg, "--") || !strings.Contains(arg, "=") {
			return invocation{}, fmt.Errorf("invalid argument %q; inputs must use --key=value", arg)
		}
		parts := strings.SplitN(strings.TrimPrefix(arg, "--"), "=", 2)
		if !validInputKey(parts[0]) {
			return invocation{}, fmt.Errorf("invalid input name %q", parts[0])
		}
		inputs[strings.ToUpper(parts[0])] = parts[1]
	}
	return invocation{owner: owner, repo: repo, ref: ref, runtime: runtime, inputs: inputs}, nil
}

func parseIdentifier(value string) (owner, repo, ref string, err error) {
	at := strings.LastIndex(value, "@")
	if at <= 0 || at == len(value)-1 {
		return "", "", "", fmt.Errorf("invalid snippet identifier %q; expected owner/repo@ref", value)
	}
	path := strings.Split(value[:at], "/")
	if len(path) != 2 || !validPathPart(path[0]) || !validPathPart(path[1]) {
		return "", "", "", fmt.Errorf("invalid snippet identifier %q; expected owner/repo@ref", value)
	}
	return path[0], path[1], value[at+1:], nil
}

func validPathPart(value string) bool {
	for _, char := range value {
		if !(char == '.' || char == '_' || char == '-' || char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9') {
			return false
		}
	}
	return value != "" && value != "." && value != ".."
}

func validCommit(value string) bool {
	if len(value) < 7 || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

func validInputKey(key string) bool {
	for i, char := range key {
		if !(char == '_' || char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || i > 0 && char >= '0' && char <= '9') {
			return false
		}
	}
	return key != ""
}

func environment(inputs map[string]string) []string {
	environment := make([]string, 0, len(os.Environ())+len(inputs))
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		if _, overridden := inputs[strings.TrimPrefix(name, "INPUTS_")]; name != strings.TrimPrefix(name, "INPUTS_") && overridden {
			continue
		}
		environment = append(environment, value)
	}
	for key, value := range inputs {
		environment = append(environment, "INPUTS_"+key+"="+value)
	}
	return environment
}

func formatBytes(size int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	value := float64(size)
	index := 0
	for value >= 1024 && index < len(units)-1 {
		value /= 1024
		index++
	}
	if index == 0 {
		return fmt.Sprintf("%d %s", size, units[index])
	}
	return fmt.Sprintf("%.1f %s", value, units[index])
}

func isHelp(arg string) bool {
	return arg == "--help" || arg == "-h" || arg == "help"
}

func fail(code int, format string, arguments ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", arguments...)
	os.Exit(code)
}
