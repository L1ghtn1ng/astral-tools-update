package updater

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const uvInstallCommand = "curl -LsSf https://astral.sh/uv/install.sh | sh"

var toolNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type Runner interface {
	LookPath(file string) (string, error)
	Run(name string, args ...string) error
	RunShell(command string) error
}

type Env interface {
	HomeDir() (string, error)
	FileExists(path string) bool
}

type Updater struct {
	Runner Runner
	Env    Env
	Log    *log.Logger
}

func (updater *Updater) logger() *log.Logger {
	if updater != nil && updater.Log != nil {
		return updater.Log
	}
	return log.New(os.Stderr, "", 0)
}

func (updater *Updater) validate() error {
	if updater == nil {
		return errors.New("updater is nil")
	}
	if updater.Runner == nil {
		return errors.New("runner is nil")
	}
	if updater.Env == nil {
		return errors.New("env is nil")
	}
	return nil
}

func (updater *Updater) Update(tools []string, noSelfUpdate bool) error {
	if err := updater.validate(); err != nil {
		return err
	}

	if len(tools) == 0 {
		return errors.New("no tools specified")
	}
	for _, tool := range tools {
		if !toolNameRE.MatchString(tool) {
			return fmt.Errorf("invalid tool name %q", tool)
		}
	}

	uvPath, err := updater.ensureUVInstalled()
	if err != nil {
		return err
	}

	if !noSelfUpdate {
		updater.logger().Printf("INFO: Updating uv...")
		if err := updater.Runner.Run(uvPath, "self", "update"); err != nil {
			return fmt.Errorf("uv self update failed: %w", err)
		}
	}

	for _, tool := range tools {
		if toolPath, ok := updater.getToolPath(tool); ok {
			updater.logger().Printf("INFO: %s found at %s, upgrading...", tool, toolPath)
			if err := updater.Runner.Run(uvPath, "tool", "upgrade", tool); err != nil {
				return fmt.Errorf("uv tool upgrade %s failed: %w", tool, err)
			}
			continue
		}

		updater.logger().Printf("INFO: %s not found, installing...", tool)
		if err := updater.Runner.Run(uvPath, "tool", "install", tool+"@latest"); err != nil {
			return fmt.Errorf("uv tool install %s@latest failed: %w", tool, err)
		}
	}

	updater.logger().Printf("INFO: All tools updated successfully!")
	return nil
}

func (updater *Updater) ensureUVInstalled() (string, error) {
	if err := updater.validate(); err != nil {
		return "", err
	}

	if uvPath, ok := updater.getToolPath("uv"); ok {
		updater.logger().Printf("INFO: uv found at %s", uvPath)
		return uvPath, nil
	}

	updater.logger().Printf("INFO: uv not found, installing...")
	if runtime.GOOS == "windows" {
		return "", errors.New("automatic uv installation is not supported on Windows")
	}
	if err := updater.Runner.RunShell(uvInstallCommand); err != nil {
		return "", fmt.Errorf("failed to install uv: %w", err)
	}

	if uvPath, ok := updater.getToolPath("uv"); ok {
		return uvPath, nil
	}

	home, err := updater.Env.HomeDir()
	if err != nil {
		return "", fmt.Errorf("uv installation appeared to succeed but home dir could not be determined: %w", err)
	}
	fallback := filepath.Join(home, ".local", "bin", "uv")
	if !updater.Env.FileExists(fallback) {
		return "", errors.New("uv installation appeared to succeed but executable was not found")
	}
	return fallback, nil
}

func (updater *Updater) getToolPath(tool string) (string, bool) {
	if updater == nil || updater.Runner == nil || updater.Env == nil {
		return "", false
	}

	toolInPath, err := updater.Runner.LookPath(tool)
	if err == nil && toolInPath != "" {
		return toolInPath, true
	}

	home, err := updater.Env.HomeDir()
	if err != nil {
		return "", false
	}
	defaultPath := filepath.Join(home, ".local", "bin", tool)
	if updater.Env.FileExists(defaultPath) {
		return defaultPath, true
	}
	return "", false
}

type realRunner struct{}

func (realRunner) LookPath(file string) (string, error) { return exec.LookPath(file) }

func (realRunner) Run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return wrapCommandError(name, args, err)
	}
	return nil
}

func (realRunner) RunShell(command string) error {
	name, args := "sh", []string{"-c", command}
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return wrapCommandError(name, args, err)
	}
	return nil
}

func wrapCommandError(name string, args []string, err error) error {
	argv := append([]string{name}, args...)
	command := strings.Join(argv, " ")
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		return fmt.Errorf("command %q failed with exit code %d: %w", command, exitErr.ExitCode(), err)
	}
	return fmt.Errorf("command %q failed: %w", command, err)
}

type realEnv struct{}

func (realEnv) HomeDir() (string, error) { return os.UserHomeDir() }

func (realEnv) FileExists(path string) bool {
	st, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !st.IsDir()
}

func NewReal(logger *log.Logger) *Updater {
	if logger == nil {
		logger = log.New(os.Stderr, "", 0)
	}
	return new(Updater{Runner: realRunner{}, Env: realEnv{}, Log: logger})
}
