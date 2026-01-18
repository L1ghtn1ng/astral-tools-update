package updater

import (
	"errors"
	"log"
	"strings"
	"testing"
)

type fakeRunner struct {
	lookPath map[string]string
	runErr   map[string]error
	calls    []string
}

func (runner *fakeRunner) LookPath(file string) (string, error) {
	if path, ok := runner.lookPath[file]; ok {
		return path, nil
	}
	return "", errors.New("not found")
}

func (runner *fakeRunner) Run(name string, args ...string) error {
	key := name + " " + strings.Join(args, " ")
	runner.calls = append(runner.calls, key)
	if err, ok := runner.runErr[key]; ok {
		return err
	}
	return nil
}

func (runner *fakeRunner) RunShell(command string) error {
	runner.calls = append(runner.calls, "sh -c "+command)
	if err, ok := runner.runErr["sh -c "+command]; ok {
		return err
	}
	return nil
}

type fakeEnv struct {
	home     string
	exists   map[string]bool
	existsFn func(string) bool
	err      error
}

func (environment fakeEnv) HomeDir() (string, error) {
	if environment.err != nil {
		return "", environment.err
	}
	return environment.home, nil
}

func (environment fakeEnv) FileExists(path string) bool {
	if environment.existsFn != nil {
		return environment.existsFn(path)
	}
	return environment.exists[path]
}

func TestUpdate_UsesUvFromPATH(test *testing.T) {
	runner := &fakeRunner{lookPath: map[string]string{"uv": "/usr/bin/uv"}}
	environment := fakeEnv{home: "/home/x", exists: map[string]bool{}}
	var logs strings.Builder

	toolUpdater := &Updater{Runner: runner, Env: environment, Log: log.New(&logs, "", 0)}
	if err := toolUpdater.Update([]string{"ruff"}, false); err != nil {
		test.Fatalf("expected nil error, got %v", err)
	}

	if got := strings.Join(runner.calls, "\n"); !strings.Contains(got, "/usr/bin/uv self update") {
		test.Fatalf("expected self update call, got calls:\n%s", got)
	}
	if got := strings.Join(runner.calls, "\n"); !strings.Contains(got, "/usr/bin/uv tool install ruff@latest") {
		test.Fatalf("expected tool install call, got calls:\n%s", got)
	}
}

func TestUpdate_SkipsSelfUpdate(test *testing.T) {
	runner := &fakeRunner{lookPath: map[string]string{"uv": "/usr/bin/uv"}}
	environment := fakeEnv{home: "/home/x", exists: map[string]bool{}}

	toolUpdater := &Updater{Runner: runner, Env: environment, Log: log.New(&strings.Builder{}, "", 0)}
	if err := toolUpdater.Update([]string{"ty"}, true); err != nil {
		test.Fatalf("expected nil error, got %v", err)
	}

	got := strings.Join(runner.calls, "\n")
	if strings.Contains(got, "self update") {
		test.Fatalf("expected no self update call, got calls:\n%s", got)
	}
}

func TestEnsureUVInstalled_InstallsWhenMissingThenUsesFallback(test *testing.T) {
	runner := &fakeRunner{lookPath: map[string]string{}}
	existsCalls := 0
	environment := fakeEnv{
		home: "/home/x",
		existsFn: func(path string) bool {
			if path != "/home/x/.local/bin/uv" {
				return false
			}
			if existsCalls == 0 {
				existsCalls++
				return false
			}
			return true
		},
	}

	toolUpdater := &Updater{Runner: runner, Env: environment, Log: log.New(&strings.Builder{}, "", 0)}
	uvPath, err := toolUpdater.ensureUVInstalled()
	if err != nil {
		test.Fatalf("expected nil error, got %v", err)
	}
	if uvPath != "/home/x/.local/bin/uv" {
		test.Fatalf("expected fallback uv path, got %q", uvPath)
	}

	got := strings.Join(runner.calls, "\n")
	if !strings.Contains(got, "sh -c "+uvInstallCommand) {
		test.Fatalf("expected install command, got calls:\n%s", got)
	}
}

func TestUpdate_RejectsInvalidToolName(test *testing.T) {
	runner := &fakeRunner{lookPath: map[string]string{"uv": "/usr/bin/uv"}}
	environment := fakeEnv{home: "/home/x", exists: map[string]bool{}}

	toolUpdater := &Updater{Runner: runner, Env: environment, Log: log.New(&strings.Builder{}, "", 0)}
	if err := toolUpdater.Update([]string{"bad tool"}, false); err == nil {
		test.Fatalf("expected error")
	}
}

func TestUpdate_AllowsNilLogger(test *testing.T) {
	runner := &fakeRunner{lookPath: map[string]string{"uv": "/usr/bin/uv"}}
	environment := fakeEnv{home: "/home/x", exists: map[string]bool{}}

	toolUpdater := &Updater{Runner: runner, Env: environment, Log: nil}
	if err := toolUpdater.Update([]string{"ruff"}, true); err != nil {
		test.Fatalf("expected nil error, got %v", err)
	}
}

func TestUpdate_ReturnsErrorWhenRunnerNil(test *testing.T) {
	environment := fakeEnv{home: "/home/x", exists: map[string]bool{}}

	toolUpdater := &Updater{Runner: nil, Env: environment, Log: nil}
	if err := toolUpdater.Update([]string{"ruff"}, true); err == nil {
		test.Fatalf("expected error")
	}
}

func TestEnsureUVInstalled_ReturnsErrorWhenEnvNil(test *testing.T) {
	runner := &fakeRunner{lookPath: map[string]string{"uv": "/usr/bin/uv"}}

	toolUpdater := &Updater{Runner: runner, Env: nil, Log: nil}
	if _, err := toolUpdater.ensureUVInstalled(); err == nil {
		test.Fatalf("expected error")
	}
}
