package selfupdate

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultOwner         = "L1ghtn1ng"
	defaultRepo          = "astral-tools-update"
	defaultOSReleasePath = "/etc/os-release"
	binaryName           = "astral-update"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type CommandRunner interface {
	Run(name string, args ...string) error
}

type FileSystem interface {
	ReadFile(name string) ([]byte, error)
	CreateTemp(dir, pattern string) (*os.File, error)
	Open(name string) (*os.File, error)
	Chmod(name string, mode os.FileMode) error
	Rename(oldpath, newpath string) error
	Remove(name string) error
}

type Updater struct {
	CurrentVersion string
	Owner          string
	Repo           string
	Client         HTTPClient
	Runner         CommandRunner
	FS             FileSystem
	Log            *log.Logger
	GOOS           string
	GOARCH         string
	EUID           int
	OSReleasePath  string
	ExecutablePath string
	TempDir        string
}

type Result struct {
	LatestVersion  string
	AssetName      string
	UpdateFound    bool
	InstallStarted bool
	Installed      bool
}

type githubRelease struct {
	TagName    string        `json:"tag_name"`
	Draft      bool          `json:"draft"`
	Prerelease bool          `json:"prerelease"`
	Assets     []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type realRunner struct{}

func (realRunner) Run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		argv := append([]string{name}, args...)
		return fmt.Errorf("command %q failed: %w", strings.Join(argv, " "), err)
	}
	return nil
}

type osFileSystem struct{}

func (osFileSystem) ReadFile(name string) ([]byte, error) { return os.ReadFile(name) }
func (osFileSystem) CreateTemp(dir, pattern string) (*os.File, error) {
	return os.CreateTemp(dir, pattern)
}
func (osFileSystem) Open(name string) (*os.File, error) { return os.Open(name) }
func (osFileSystem) Chmod(name string, mode os.FileMode) error {
	return os.Chmod(name, mode)
}
func (osFileSystem) Rename(oldpath, newpath string) error { return os.Rename(oldpath, newpath) }
func (osFileSystem) Remove(name string) error             { return os.Remove(name) }

func New(currentVersion string, logger *log.Logger) *Updater {
	executablePath, _ := os.Executable()
	if logger == nil {
		logger = log.New(os.Stderr, "", 0)
	}
	return &Updater{
		CurrentVersion: currentVersion,
		Owner:          defaultOwner,
		Repo:           defaultRepo,
		Client:         &http.Client{Timeout: 15 * time.Second},
		Runner:         realRunner{},
		FS:             osFileSystem{},
		Log:            logger,
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
		EUID:           os.Geteuid(),
		OSReleasePath:  defaultOSReleasePath,
		ExecutablePath: executablePath,
		TempDir:        os.TempDir(),
	}
}

func (updater *Updater) CheckAndInstall(ctx context.Context) (Result, error) {
	var result Result
	if err := updater.validate(); err != nil {
		return result, err
	}

	release, err := updater.fetchLatestRelease(ctx)
	if err != nil {
		return result, err
	}
	if release.Draft || release.Prerelease {
		return result, fmt.Errorf("latest release %q is not a stable release", release.TagName)
	}
	result.LatestVersion = release.TagName

	newer, err := isNewerVersion(updater.CurrentVersion, release.TagName)
	if err != nil {
		return result, err
	}
	if !newer {
		updater.logger().Printf("INFO: astral-update is current (%s)", updater.CurrentVersion)
		return result, nil
	}
	result.UpdateFound = true

	asset, err := updater.selectAsset(release.Assets)
	if err != nil {
		return result, err
	}
	result.AssetName = asset.Name
	updater.logger().Printf("INFO: astral-update %s is available; installing %s", release.TagName, asset.Name)

	downloadedPath, err := updater.downloadAsset(ctx, asset)
	if err != nil {
		return result, err
	}
	defer func() { _ = updater.FS.Remove(downloadedPath) }()

	result.InstallStarted = true
	if err := updater.installAsset(downloadedPath, asset.Name); err != nil {
		return result, err
	}
	result.Installed = true
	updater.logger().Printf("INFO: astral-update updated to %s", release.TagName)
	return result, nil
}

func (updater *Updater) validate() error {
	if updater == nil {
		return errors.New("self updater is nil")
	}
	if strings.TrimSpace(updater.CurrentVersion) == "" {
		return errors.New("current version is empty")
	}
	if updater.Client == nil {
		return errors.New("http client is nil")
	}
	if updater.Runner == nil {
		return errors.New("runner is nil")
	}
	if updater.FS == nil {
		return errors.New("filesystem is nil")
	}
	if strings.TrimSpace(updater.Owner) == "" {
		updater.Owner = defaultOwner
	}
	if strings.TrimSpace(updater.Repo) == "" {
		updater.Repo = defaultRepo
	}
	if strings.TrimSpace(updater.GOOS) == "" {
		updater.GOOS = runtime.GOOS
	}
	if strings.TrimSpace(updater.GOARCH) == "" {
		updater.GOARCH = runtime.GOARCH
	}
	if strings.TrimSpace(updater.OSReleasePath) == "" {
		updater.OSReleasePath = defaultOSReleasePath
	}
	if strings.TrimSpace(updater.TempDir) == "" {
		updater.TempDir = os.TempDir()
	}
	return nil
}

func (updater *Updater) logger() *log.Logger {
	if updater != nil && updater.Log != nil {
		return updater.Log
	}
	return log.New(os.Stderr, "", 0)
}

func (updater *Updater) fetchLatestRelease(ctx context.Context) (githubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", updater.Owner, updater.Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return githubRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "astral-update")

	resp, err := updater.Client.Do(req)
	if err != nil {
		return githubRelease{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		limited, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return githubRelease{}, fmt.Errorf("github latest release request failed: %s: %s", resp.Status, strings.TrimSpace(string(limited)))
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return githubRelease{}, err
	}
	if strings.TrimSpace(release.TagName) == "" {
		return githubRelease{}, errors.New("github latest release response did not include tag_name")
	}
	return release, nil
}

func (updater *Updater) selectAsset(assets []githubAsset) (githubAsset, error) {
	formatOrder := updater.preferredFormats()
	for _, format := range formatOrder {
		var matches []githubAsset
		for _, asset := range assets {
			if assetMatches(asset.Name, updater.GOARCH, format) {
				matches = append(matches, asset)
			}
		}
		if len(matches) == 0 {
			continue
		}
		sort.Slice(matches, func(left, right int) bool {
			return matches[left].Name < matches[right].Name
		})
		return matches[0], nil
	}
	return githubAsset{}, fmt.Errorf("no release asset matched %s/%s with formats %s", updater.GOOS, updater.GOARCH, strings.Join(formatOrder, ", "))
}

func (updater *Updater) preferredFormats() []string {
	if updater.GOOS != "linux" {
		return []string{}
	}
	native := updater.detectLinuxPackageFormat()
	if native == "" {
		return []string{"tar.gz"}
	}
	return []string{native, "tar.gz"}
}

func (updater *Updater) detectLinuxPackageFormat() string {
	content, err := updater.FS.ReadFile(updater.OSReleasePath)
	if err != nil {
		return ""
	}
	values := parseOSRelease(string(content))
	ids := strings.FieldsSeq(values["ID"] + " " + values["ID_LIKE"])
	for id := range ids {
		switch strings.ToLower(id) {
		case "debian", "ubuntu":
			return "deb"
		case "rhel", "fedora", "centos", "rocky", "almalinux", "suse", "opensuse":
			return "rpm"
		}
	}
	return ""
}

func (updater *Updater) downloadAsset(ctx context.Context, asset githubAsset) (string, error) {
	if strings.TrimSpace(asset.BrowserDownloadURL) == "" {
		return "", fmt.Errorf("release asset %q does not include a download URL", asset.Name)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.BrowserDownloadURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "astral-update")

	resp, err := updater.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %q failed: %s", asset.Name, resp.Status)
	}

	tmp, err := updater.FS.CreateTemp(updater.TempDir, "astral-update-*"+assetExtension(asset.Name))
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		_ = tmp.Close()
		_ = updater.FS.Remove(tmpPath)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = updater.FS.Remove(tmpPath)
		return "", err
	}
	return tmpPath, nil
}

func (updater *Updater) installAsset(path, assetName string) error {
	switch {
	case strings.HasSuffix(assetName, ".deb"):
		return updater.runInstallCommand("dpkg", "-i", path)
	case strings.HasSuffix(assetName, ".rpm"):
		return updater.runInstallCommand("rpm", "-Uvh", path)
	case strings.HasSuffix(assetName, ".tar.gz"):
		return updater.replaceFromArchive(path)
	default:
		return fmt.Errorf("unsupported release asset format: %s", assetName)
	}
}

func (updater *Updater) runInstallCommand(name string, args ...string) error {
	if updater.EUID == 0 {
		return updater.Runner.Run(name, args...)
	}
	sudoArgs := append([]string{name}, args...)
	return updater.Runner.Run("sudo", sudoArgs...)
}

func (updater *Updater) replaceFromArchive(path string) error {
	if updater.ExecutablePath == "" {
		return errors.New("current executable path could not be determined")
	}
	archive, err := updater.FS.Open(path)
	if err != nil {
		return err
	}
	defer archive.Close()

	gzipReader, err := gzip.NewReader(archive)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	dir := filepath.Dir(updater.ExecutablePath)
	tmp, err := updater.FS.CreateTemp(dir, ".astral-update-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	wroteBinary := false
	defer func() {
		if !wroteBinary {
			_ = updater.FS.Remove(tmpPath)
		}
	}()

	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			_ = tmp.Close()
			return err
		}
		if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != binaryName {
			continue
		}
		if _, err := io.Copy(tmp, tarReader); err != nil {
			_ = tmp.Close()
			return err
		}
		if err := tmp.Close(); err != nil {
			return err
		}
		if err := updater.FS.Chmod(tmpPath, 0o755); err != nil {
			return err
		}
		if err := updater.FS.Rename(tmpPath, updater.ExecutablePath); err != nil {
			return err
		}
		wroteBinary = true
		return nil
	}

	_ = tmp.Close()
	return fmt.Errorf("archive did not contain %s", binaryName)
}

func parseOSRelease(content string) map[string]string {
	values := map[string]string{}
	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		values[strings.TrimSpace(key)] = value
	}
	return values
}

func assetMatches(name, goarch, format string) bool {
	lowerName := strings.ToLower(name)
	if !assetHasFormat(lowerName, format) {
		return false
	}
	for _, arch := range archAliases(goarch) {
		if strings.Contains(lowerName, arch) {
			return true
		}
	}
	return false
}

func assetHasFormat(name, format string) bool {
	switch format {
	case "tar.gz":
		return strings.HasSuffix(name, ".tar.gz")
	case "deb":
		return strings.HasSuffix(name, ".deb")
	case "rpm":
		return strings.HasSuffix(name, ".rpm")
	default:
		return false
	}
}

func archAliases(goarch string) []string {
	switch goarch {
	case "amd64":
		return []string{"amd64", "x86_64"}
	case "arm64":
		return []string{"arm64", "aarch64"}
	default:
		return []string{goarch}
	}
}

func assetExtension(name string) string {
	switch {
	case strings.HasSuffix(name, ".tar.gz"):
		return ".tar.gz"
	case strings.HasSuffix(name, ".deb"):
		return ".deb"
	case strings.HasSuffix(name, ".rpm"):
		return ".rpm"
	default:
		return ""
	}
}

func isNewerVersion(current, candidate string) (bool, error) {
	currentParts, err := parseVersion(current)
	if err != nil {
		return false, fmt.Errorf("current version %q is invalid: %w", current, err)
	}
	candidateParts, err := parseVersion(candidate)
	if err != nil {
		return false, fmt.Errorf("candidate version %q is invalid: %w", candidate, err)
	}
	for index := range currentParts {
		if candidateParts[index] > currentParts[index] {
			return true, nil
		}
		if candidateParts[index] < currentParts[index] {
			return false, nil
		}
	}
	return false, nil
}

func parseVersion(version string) ([3]int, error) {
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, "refs/tags/")
	version = strings.TrimPrefix(version, "v")
	version, _, _ = strings.Cut(version, "-")
	pieces := strings.Split(version, ".")
	if len(pieces) != 3 {
		return [3]int{}, errors.New("expected major.minor.patch")
	}
	var parts [3]int
	for index, piece := range pieces {
		value, err := strconv.Atoi(piece)
		if err != nil {
			return [3]int{}, err
		}
		parts[index] = value
	}
	return parts, nil
}
