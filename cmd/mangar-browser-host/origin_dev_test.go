//go:build !store

package main

import (
	"path/filepath"
	"testing"
)

func TestDevelopmentApplicationPathDoesNotChangeDefaultOrStoreBuild(t *testing.T) {
	prior := developmentAppPath
	t.Cleanup(func() { developmentAppPath = prior })
	self := filepath.Join(t.TempDir(), "mangar-browser-host.exe")
	developmentAppPath = ""
	if applicationPath(self) != filepath.Join(filepath.Dir(self), "YTDLP-GUI-Mangar.exe") {
		t.Fatal("default launch path changed")
	}
	developmentAppPath = filepath.Join(t.TempDir(), "YTDLP-GUI-Mangar.exe")
	if applicationPath(self) != developmentAppPath {
		t.Fatal("installed dev GUI not selected")
	}
	developmentAppPath = "relative/YTDLP-GUI-Mangar.exe"
	if applicationPath(self) == developmentAppPath {
		t.Fatal("relative dev app accepted")
	}
	developmentAppPath = filepath.Join(t.TempDir(), "other.exe")
	if applicationPath(self) == developmentAppPath {
		t.Fatal("unrelated app accepted")
	}
}
