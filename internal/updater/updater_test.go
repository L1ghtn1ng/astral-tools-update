package updater

import (
	"errors"
	"log"
	"runtime"
	"strings"
	"testing"
)

type fakeRunner struct {
	lookPath map[string]string
	output   map[string]string
	runErr   map[string]error
	calls    []string
}

func (runner *fakeRunner) LookPath(file string) (string, error) {
	if path, ok := runner.lookPath[file]; ok {
		return path, nil
	}
	return "", errors.New("not found")
}

func (runner *fakeRunner) Output(name string, args ...string) (string, error) {
	key := name + " " + strings.Join(args, " ")
	runner.calls = append(runner.calls, key)
	if err, ok := runner.runErr[key]; ok {
		return "", err
	}
	if output, ok := runner.output[key]; ok {
		return output, nil
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
	runner := &fakeRunner{lookPath: map[string]string{"uv": "/usr/bin/uv", "ruff": "/usr/bin/ruff"}}
	environment := fakeEnv{home: "/home/x", exists: map[string]bool{}}
	var logs strings.Builder

	toolUpdater := &Updater{Runner: runner, Env: environment, Log: log.New(&logs, "", 0)}
	if err := toolUpdater.Update([]string{"ruff"}, false); err != nil {
		test.Fatalf("expected nil error, got %v", err)
	}

	if got := strings.Join(runner.calls, "\n"); !strings.Contains(got, "/usr/bin/uv self update") {
		test.Fatalf("expected self update call, got calls:\n%s", got)
	}
	if got := strings.Join(runner.calls, "\n"); !strings.Contains(got, "/usr/bin/uv tool upgrade ruff") {
		test.Fatalf("expected tool upgrade call, got calls:\n%s", got)
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
	if !strings.Contains(got, "/usr/bin/uv tool install ty@latest") {
		test.Fatalf("expected tool install call, got calls:\n%s", got)
	}
}

func TestUpdate_UsesFallbackToolPathForUpgrade(test *testing.T) {
	runner := &fakeRunner{lookPath: map[string]string{"uv": "/usr/bin/uv"}}
	environment := fakeEnv{
		home:   "/home/x",
		exists: map[string]bool{"/home/x/.local/bin/ty": true},
	}

	toolUpdater := &Updater{Runner: runner, Env: environment, Log: log.New(&strings.Builder{}, "", 0)}
	if err := toolUpdater.Update([]string{"ty"}, true); err != nil {
		test.Fatalf("expected nil error, got %v", err)
	}

	got := strings.Join(runner.calls, "\n")
	if !strings.Contains(got, "/usr/bin/uv tool upgrade ty") {
		test.Fatalf("expected tool upgrade call, got calls:\n%s", got)
	}
}

func TestUpdate_UsesUvToolBinDirForUpgrade(test *testing.T) {
	runner := &fakeRunner{
		lookPath: map[string]string{"uv": "/usr/bin/uv"},
		output:   map[string]string{"/usr/bin/uv tool dir --bin": "/opt/uv-bin\n"},
	}
	environment := fakeEnv{
		home:   "/home/x",
		exists: map[string]bool{"/opt/uv-bin/ty": true},
	}

	toolUpdater := &Updater{Runner: runner, Env: environment, Log: log.New(&strings.Builder{}, "", 0)}
	if err := toolUpdater.Update([]string{"ty"}, true); err != nil {
		test.Fatalf("expected nil error, got %v", err)
	}

	got := strings.Join(runner.calls, "\n")
	if !strings.Contains(got, "/usr/bin/uv tool dir --bin") {
		test.Fatalf("expected uv tool dir lookup, got calls:\n%s", got)
	}
	if !strings.Contains(got, "/usr/bin/uv tool upgrade ty") {
		test.Fatalf("expected tool upgrade call, got calls:\n%s", got)
	}
	if strings.Contains(got, "/usr/bin/uv tool install ty@latest") {
		test.Fatalf("expected no install call, got calls:\n%s", got)
	}
}

func TestUpdate_RefreshesUvToolBinDirAfterSelfUpdate(test *testing.T) {
	runner := &fakeRunner{
		lookPath: map[string]string{"uv": "/usr/bin/uv"},
		output:   map[string]string{"/usr/bin/uv tool dir --bin": "/opt/uv-bin\n"},
	}
	environment := fakeEnv{
		home:   "/home/x",
		exists: map[string]bool{"/opt/uv-bin/ty": true},
	}

	toolUpdater := &Updater{Runner: runner, Env: environment, Log: log.New(&strings.Builder{}, "", 0)}
	if err := toolUpdater.Update([]string{"ty"}, false); err != nil {
		test.Fatalf("expected nil error, got %v", err)
	}

	selfUpdateCall := "/usr/bin/uv self update"
	toolDirCall := "/usr/bin/uv tool dir --bin"
	got := strings.Join(runner.calls, "\n")
	selfUpdateIndex := strings.Index(got, selfUpdateCall)
	toolDirIndex := strings.Index(got, toolDirCall)
	if selfUpdateIndex == -1 {
		test.Fatalf("expected self update call, got calls:\n%s", got)
	}
	if toolDirIndex == -1 {
		test.Fatalf("expected uv tool dir lookup, got calls:\n%s", got)
	}
	if selfUpdateIndex > toolDirIndex {
		test.Fatalf("expected self update before uv tool dir lookup, got calls:\n%s", got)
	}
	if !strings.Contains(got, "/usr/bin/uv tool upgrade ty") {
		test.Fatalf("expected tool upgrade call, got calls:\n%s", got)
	}
}

func TestUpdate_UsesLegacyFallbackWhenUvToolBinDirFails(test *testing.T) {
	runner := &fakeRunner{
		lookPath: map[string]string{"uv": "/usr/bin/uv"},
		runErr:   map[string]error{"/usr/bin/uv tool dir --bin": errors.New("boom")},
	}
	environment := fakeEnv{
		home:   "/home/x",
		exists: map[string]bool{"/home/x/.local/bin/ty": true},
	}

	toolUpdater := &Updater{Runner: runner, Env: environment, Log: log.New(&strings.Builder{}, "", 0)}
	if err := toolUpdater.Update([]string{"ty"}, true); err != nil {
		test.Fatalf("expected nil error, got %v", err)
	}

	got := strings.Join(runner.calls, "\n")
	if !strings.Contains(got, "/usr/bin/uv tool dir --bin") {
		test.Fatalf("expected uv tool dir lookup, got calls:\n%s", got)
	}
	if !strings.Contains(got, "/usr/bin/uv tool upgrade ty") {
		test.Fatalf("expected tool upgrade call, got calls:\n%s", got)
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

func TestRealRunner_RunShell_IncludesExitCode(test *testing.T) {
	if runtime.GOOS == "windows" {
		test.Skip("shell exit code behavior differs on Windows")
	}

	runner := realRunner{}
	err := runner.RunShell("exit 7")
	if err == nil {
		test.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "exit code 7") {
		test.Fatalf("expected wrapped error with exit code, got: %v", err)
	}
}
