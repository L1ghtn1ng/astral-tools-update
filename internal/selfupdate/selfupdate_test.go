package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeHTTPClient struct {
	responses map[string]fakeHTTPResponse
	err       error
	requests  []string
}

type fakeHTTPResponse struct {
	status int
	body   string
	bytes  []byte
}

func (client *fakeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	client.requests = append(client.requests, req.URL.String())
	if client.err != nil {
		return nil, client.err
	}
	response, ok := client.responses[req.URL.String()]
	if !ok {
		return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
	}
	body := []byte(response.body)
	if response.bytes != nil {
		body = response.bytes
	}
	status := response.status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}, nil
}

type fakeRunner struct {
	err   error
	calls []string
}

func (runner *fakeRunner) Run(name string, args ...string) error {
	runner.calls = append(runner.calls, name+" "+strings.Join(args, " "))
	return runner.err
}

func TestCheckAndInstall_NoUpdateForCurrentVersion(t *testing.T) {
	client := &fakeHTTPClient{responses: map[string]fakeHTTPResponse{
		"https://api.github.com/repos/L1ghtn1ng/astral-tools-update/releases/latest": {
			body: `{"tag_name":"v1.0.1","assets":[]}`,
		},
	}}
	runner := &fakeRunner{}
	updater := testUpdater(t, client, runner, "1.0.1")

	result, err := updater.CheckAndInstall(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result.UpdateFound {
		t.Fatalf("expected no update, got result: %+v", result)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("expected no install command, got %v", runner.calls)
	}
}

func TestIsNewerVersion_AcceptsVPrefixedAndPlainVersions(t *testing.T) {
	newer, err := isNewerVersion("1.0.1", "v1.0.2")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !newer {
		t.Fatalf("expected v1.0.2 to be newer than 1.0.1")
	}

	newer, err = isNewerVersion("v1.0.2", "1.0.1")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if newer {
		t.Fatalf("expected 1.0.1 not to be newer than v1.0.2")
	}
}

func TestCheckAndInstall_IgnoresPrerelease(t *testing.T) {
	client := &fakeHTTPClient{responses: map[string]fakeHTTPResponse{
		"https://api.github.com/repos/L1ghtn1ng/astral-tools-update/releases/latest": {
			body: `{"tag_name":"v1.0.2","prerelease":true,"assets":[]}`,
		},
	}}
	updater := testUpdater(t, client, &fakeRunner{}, "1.0.1")

	result, err := updater.CheckAndInstall(context.Background())
	if err == nil {
		t.Fatalf("expected prerelease error")
	}
	if result.InstallStarted {
		t.Fatalf("expected install not to start")
	}
}

func TestSelectAsset_LinuxAMD64PrefersDebOnDebian(t *testing.T) {
	updater := testUpdater(t, nil, nil, "1.0.1")
	updater.GOOS = "linux"
	updater.GOARCH = "amd64"
	updater.ExecutablePath = packageExecutablePath
	updater.OSReleasePath = writeFile(t, updater.TempDir, "os-release", "ID=ubuntu\nID_LIKE=debian\n")

	asset, err := updater.selectAsset([]githubAsset{
		{Name: "astral-update_1.0.2_linux_arm64.deb"},
		{Name: "astral-update_1.0.2_linux_amd64.tar.gz"},
		{Name: "astral-update_1.0.2_linux_amd64.deb"},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if asset.Name != "astral-update_1.0.2_linux_amd64.deb" {
		t.Fatalf("expected amd64 deb, got %q", asset.Name)
	}
}

func TestSelectAsset_LinuxArm64PrefersRpmOnRHEL(t *testing.T) {
	updater := testUpdater(t, nil, nil, "1.0.1")
	updater.GOOS = "linux"
	updater.GOARCH = "arm64"
	updater.ExecutablePath = packageExecutablePath
	updater.OSReleasePath = writeFile(t, updater.TempDir, "os-release", "ID=rocky\nID_LIKE=\"rhel fedora\"\n")

	asset, err := updater.selectAsset([]githubAsset{
		{Name: "astral-update_1.0.2_linux_x86_64.rpm"},
		{Name: "astral-update_1.0.2_linux_aarch64.rpm"},
		{Name: "astral-update_1.0.2_linux_arm64.tar.gz"},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if asset.Name != "astral-update_1.0.2_linux_aarch64.rpm" {
		t.Fatalf("expected aarch64 rpm, got %q", asset.Name)
	}
}

func TestSelectAsset_FallsBackToTarWhenNativePackageMissing(t *testing.T) {
	updater := testUpdater(t, nil, nil, "1.0.1")
	updater.GOOS = "linux"
	updater.GOARCH = "amd64"
	updater.ExecutablePath = packageExecutablePath
	updater.OSReleasePath = writeFile(t, updater.TempDir, "os-release", "ID=debian\n")

	asset, err := updater.selectAsset([]githubAsset{
		{Name: "astral-update_1.0.2_linux_arm64.deb"},
		{Name: "astral-update_1.0.2_linux_x86_64.tar.gz"},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if asset.Name != "astral-update_1.0.2_linux_x86_64.tar.gz" {
		t.Fatalf("expected x86_64 tarball fallback, got %q", asset.Name)
	}
}

func TestSelectAsset_RejectsWrongArchitecture(t *testing.T) {
	updater := testUpdater(t, nil, nil, "1.0.1")
	updater.GOOS = "linux"
	updater.GOARCH = "arm64"
	updater.OSReleasePath = writeFile(t, updater.TempDir, "os-release", "ID=debian\n")

	_, err := updater.selectAsset([]githubAsset{
		{Name: "astral-update_1.0.2_linux_amd64.deb"},
		{Name: "astral-update_1.0.2_linux_x86_64.tar.gz"},
	})
	if err == nil {
		t.Fatalf("expected no matching asset error")
	}
}

func TestDetectLinuxPackageFormat(t *testing.T) {
	tests := map[string]struct {
		content string
		want    string
	}{
		"debian":      {content: "ID=debian\n", want: "deb"},
		"ubuntu-like": {content: "ID=pop\nID_LIKE=\"ubuntu debian\"\n", want: "deb"},
		"fedora":      {content: "ID=fedora\n", want: "rpm"},
		"rhel-like":   {content: "ID=ol\nID_LIKE=\"rhel fedora\"\n", want: "rpm"},
		"unknown":     {content: "ID=arch\n", want: ""},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			updater := testUpdater(t, nil, nil, "1.0.1")
			updater.OSReleasePath = writeFile(t, updater.TempDir, "os-release", tt.content)
			if got := updater.detectLinuxPackageFormat(); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestCheckAndInstall_GitHubFailureDoesNotStartInstall(t *testing.T) {
	client := &fakeHTTPClient{err: errors.New("network down")}
	updater := testUpdater(t, client, &fakeRunner{}, "1.0.1")

	result, err := updater.CheckAndInstall(context.Background())
	if err == nil {
		t.Fatalf("expected error")
	}
	if result.InstallStarted {
		t.Fatalf("expected install not to start")
	}
}

func TestCheckAndInstall_InstallFailureReportsInstallStarted(t *testing.T) {
	runner := &fakeRunner{err: errors.New("dpkg failed")}
	client := &fakeHTTPClient{responses: map[string]fakeHTTPResponse{
		"https://api.github.com/repos/L1ghtn1ng/astral-tools-update/releases/latest": {
			body: `{"tag_name":"v1.0.2","assets":[{"name":"astral-update_1.0.2_linux_amd64.deb","browser_download_url":"https://example.test/astral.deb"}]}`,
		},
		"https://example.test/astral.deb": {body: "package"},
	}}
	updater := testUpdater(t, client, runner, "1.0.1")
	updater.ExecutablePath = packageExecutablePath
	updater.OSReleasePath = writeFile(t, updater.TempDir, "os-release", "ID=debian\n")
	updater.EUID = 1000

	result, err := updater.CheckAndInstall(context.Background())
	if err == nil {
		t.Fatalf("expected install error")
	}
	if !result.InstallStarted {
		t.Fatalf("expected install to start")
	}
	if got := strings.Join(runner.calls, "\n"); !strings.Contains(got, "sudo dpkg -i ") {
		t.Fatalf("expected sudo dpkg install, got calls:\n%s", got)
	}
}

func TestCheckAndInstall_RootRpmInstall(t *testing.T) {
	runner := &fakeRunner{}
	client := &fakeHTTPClient{responses: map[string]fakeHTTPResponse{
		"https://api.github.com/repos/L1ghtn1ng/astral-tools-update/releases/latest": {
			body: `{"tag_name":"v1.0.2","assets":[{"name":"astral-update_1.0.2_linux_aarch64.rpm","browser_download_url":"https://example.test/astral.rpm"}]}`,
		},
		"https://example.test/astral.rpm": {body: "package"},
	}}
	updater := testUpdater(t, client, runner, "1.0.1")
	updater.GOARCH = "arm64"
	updater.ExecutablePath = packageExecutablePath
	updater.OSReleasePath = writeFile(t, updater.TempDir, "os-release", "ID=fedora\n")
	updater.EUID = 0

	result, err := updater.CheckAndInstall(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.Installed {
		t.Fatalf("expected installed result")
	}
	if got := strings.Join(runner.calls, "\n"); !strings.Contains(got, "rpm -Uvh ") {
		t.Fatalf("expected rpm install, got calls:\n%s", got)
	}
	if strings.Contains(strings.Join(runner.calls, "\n"), "sudo") {
		t.Fatalf("expected root install without sudo, got %v", runner.calls)
	}
}

func TestCheckAndInstall_NonPackageInstallReplacesInvokedExecutable(t *testing.T) {
	archive := makeArchive(t, "updated binary")
	client := &fakeHTTPClient{responses: map[string]fakeHTTPResponse{
		"https://api.github.com/repos/L1ghtn1ng/astral-tools-update/releases/latest": {
			body: `{"tag_name":"v1.0.2","assets":[{"name":"astral-update_1.0.2_linux_amd64.deb","browser_download_url":"https://example.test/astral.deb"},{"name":"astral-update_1.0.2_linux_amd64.tar.gz","browser_download_url":"https://example.test/astral.tar.gz"}]}`,
		},
		"https://example.test/astral.tar.gz": {bytes: archive},
	}}
	updater := testUpdater(t, client, &fakeRunner{}, "1.0.1")
	executablePath := filepath.Join(updater.TempDir, "astral-update")
	if err := os.WriteFile(executablePath, []byte("old binary"), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}
	updater.ExecutablePath = executablePath
	updater.OSReleasePath = writeFile(t, updater.TempDir, "os-release", "ID=debian\n")

	result, err := updater.CheckAndInstall(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.Installed {
		t.Fatalf("expected installed result")
	}
	if result.AssetName != "astral-update_1.0.2_linux_amd64.tar.gz" {
		t.Fatalf("expected tarball for non-package executable, got %q", result.AssetName)
	}
	got, err := os.ReadFile(executablePath)
	if err != nil {
		t.Fatalf("read executable: %v", err)
	}
	if string(got) != "updated binary" {
		t.Fatalf("expected replaced executable, got %q", string(got))
	}
}

func testUpdater(t *testing.T, client HTTPClient, runner CommandRunner, currentVersion string) *Updater {
	t.Helper()
	if client == nil {
		client = &fakeHTTPClient{responses: map[string]fakeHTTPResponse{}}
	}
	if runner == nil {
		runner = &fakeRunner{}
	}
	return &Updater{
		CurrentVersion: currentVersion,
		Owner:          defaultOwner,
		Repo:           defaultRepo,
		Client:         client,
		Runner:         runner,
		FS:             osFileSystem{},
		Log:            log.New(io.Discard, "", 0),
		GOOS:           "linux",
		GOARCH:         "amd64",
		EUID:           1000,
		OSReleasePath:  filepath.Join(t.TempDir(), "missing-os-release"),
		ExecutablePath: filepath.Join(t.TempDir(), "astral-update"),
		TempDir:        t.TempDir(),
	}
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func makeArchive(t *testing.T, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzipWriter)
	data := []byte(content)
	if err := tarWriter.WriteHeader(&tar.Header{
		Name: binaryName,
		Mode: 0o755,
		Size: int64(len(data)),
	}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tarWriter.Write(data); err != nil {
		t.Fatalf("write tar content: %v", err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}
