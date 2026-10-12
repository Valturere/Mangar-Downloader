package main

import (
	"YTDLP-GUI-Mangar/internal/download"
	"YTDLP-GUI-Mangar/internal/updates"
	"context"
	"strings"
	"sync"
	"testing"
)

func TestAppUpdateRejectsComponentOperationAndKeepsQueueOpen(t *testing.T) {
	a := NewApp()
	a.downloadManager = download.New(download.Options{})
	a.updateController = &updates.Controller{}
	a.toolPending = true
	defer a.downloadManager.Shutdown(context.Background())
	e := a.InstallApplicationUpdate()
	if e == nil || !strings.Contains(e.Error(), "очереди") {
		t.Fatal(e)
	}
	state := a.UpdateSnapshot()
	if state.Error != e.Error() || state.Busy || a.downloadManager.Snapshot().Closing || !a.downloadManager.IdleForUpdate() {
		t.Fatal("failed preflight changed app/queue", state)
	}
}
func TestUpdateAdapterSnapshotRevisionAndInstallNotice(t *testing.T) {
	a := NewApp()
	a.updateController = &updates.Controller{}
	a.updateInstalling = true
	a.updateError = "Безопасное сообщение"
	first := a.UpdateSnapshot()
	if !first.Busy || first.Phase != "installing" || first.Error != a.updateError {
		t.Fatal(first)
	}
	var wg sync.WaitGroup
	revisions := make(chan uint64, 50)
	for range 50 {
		wg.Add(1)
		go func() { defer wg.Done(); revisions <- a.UpdateSnapshot().Revision }()
	}
	wg.Wait()
	close(revisions)
	unique := map[uint64]bool{}
	for revision := range revisions {
		if revision <= first.Revision || unique[revision] {
			t.Fatal("duplicate/stale revision")
		}
		unique[revision] = true
	}
}
