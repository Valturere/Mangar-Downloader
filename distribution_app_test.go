package main

import (
	"YTDLP-GUI-Mangar/internal/buildinfo"
	"YTDLP-GUI-Mangar/internal/distribution"
	"testing"
)

func TestApplicationVersionAndPinnedStore(t *testing.T) {
	a := NewApp()
	info := a.ApplicationInfo()
	if info.Name != "Mangar Downloader" || info.Version != buildinfo.Current() || info.StoreURL != distribution.StoreURL() {
		t.Fatal("version/identity mismatch")
	}
	if err := a.OpenExtensionStore(); err == nil {
		t.Fatal("store opened before startup")
	}
}
