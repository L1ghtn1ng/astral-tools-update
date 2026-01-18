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
		updater.logger().Printf("INFO: Installing/updating %s...", tool)
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

	if uvPath, ok := updater.getUVPath(); ok {
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

	if uvPath, ok := updater.getUVPath(); ok {
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

func (updater *Updater) getUVPath() (string, bool) {
	if updater == nil || updater.Runner == nil || updater.Env == nil {
		return "", false
	}

	uvInPath, err := updater.Runner.LookPath("uv")
	if err == nil && uvInPath != "" {
		return uvInPath, true
	}

	home, err := updater.Env.HomeDir()
	if err != nil {
		return "", false
	}
	defaultPath := filepath.Join(home, ".local", "bin", "uv")
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
	return cmd.Run()
}

func (realRunner) RunShell(command string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
	return &Updater{Runner: realRunner{}, Env: realEnv{}, Log: logger}
}
