package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseVersionConfiguration(t *testing.T) {
	if version != developmentVersion {
		t.Fatalf("development build version = %q, want %q", version, developmentVersion)
	}

	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source file")
	}
	configPath := filepath.Join(filepath.Dir(testFile), "..", "..", ".goreleaser.yaml")
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read GoReleaser configuration: %v", err)
	}
	if !strings.Contains(string(config), "-X main.version={{ .Version }}") {
		t.Fatal("GoReleaser must inject the release tag into main.version")
	}
}

func TestShouldCheckForSelfUpdate(t *testing.T) {
	tests := map[string]struct {
		disabled       bool
		currentVersion string
		want           bool
	}{
		"release build": {currentVersion: "1.2.0", want: true},
		"disabled":      {disabled: true, currentVersion: "1.2.0", want: false},
		"development":   {currentVersion: developmentVersion, want: false},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := shouldCheckForSelfUpdate(test.disabled, test.currentVersion); got != test.want {
				t.Fatalf("shouldCheckForSelfUpdate(%t, %q) = %t, want %t", test.disabled, test.currentVersion, got, test.want)
			}
		})
	}
}
