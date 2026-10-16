package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"YTDLP-GUI-Mangar/internal/apppaths"
	"YTDLP-GUI-Mangar/internal/tools"
)

type blockedToolSource struct{ entered chan struct{} }

func (s blockedToolSource) Latest(ctx context.Context, _ tools.Tool) (tools.Candidate, error) {
	close(s.entered)
	<-ctx.Done()
	return tools.Candidate{}, ctx.Err()
}
func TestComponentsAdapterDuplicateAndShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	a := NewApp()
	a.toolContext = ctx
	a.toolCancel = cancel
	a.toolErrorEvent = func(string) {}
	a.toolManager = tools.New(tools.Options{Paths: apppaths.At(t.TempDir()), Source: blockedToolSource{entered: entered}})
	if e := a.ComponentsAction("arbitrary"); e == nil {
		t.Fatal("unknown action accepted")
	}
	if e := a.ComponentsAction("install"); e != nil {
		t.Fatal(e)
	}
	if e := a.ComponentsAction("install"); e == nil {
		t.Fatal("adapter duplicate accepted")
	}
	<-entered
	done := make(chan struct{})
	go func() { a.shutdown(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not cancel and await manager")
	}
	if a.toolManager.Snapshot().Busy {
		t.Fatal("operation remains active")
	}
	if e := a.ComponentsAction("check"); e == nil {
		t.Fatal("action accepted after shutdown")
	}
}

func TestToolsStartupBeforeDOMKeepsLocalSnapshot(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	a := NewApp()
	// No Wails frontend in this context: startup must retain DTOs without
	// attempting JavaScript events before DOM/runtime initialization.
	a.newToolManager = func(o tools.Options) *tools.Manager {
		o.Source = offlineTools{}
		return tools.New(o)
	}
	a.startup(context.Background())
	a.toolWG.Wait()
	snapshot := a.ToolsSnapshot()
	if snapshot.Busy || len(snapshot.Tools) != 3 || snapshot.Error == "" || snapshot.Preparation != "failed" {
		t.Fatalf("startup snapshot: %+v", snapshot)
	}
	for _, tool := range snapshot.Tools {
		if tool.State != "not_installed" && tool.State != "failed" {
			t.Fatalf("unexpected startup state: %+v", tool)
		}
	}
	a.shutdown(context.Background())
}

type offlineTools struct{}

func (offlineTools) Latest(context.Context, tools.Tool) (tools.Candidate, error) {
	return tools.Candidate{}, errors.New("offline fixture")
}

func TestToolsStartupAfterShutdownDoesNotStartWork(t *testing.T) {
	a := NewApp()
	a.shutdown(context.Background())
	a.startup(context.Background())
	a.domReady(context.Background())
	if a.toolManager != nil || a.toolCancel != nil || a.toolPending {
		t.Fatal("startup started tools work after shutdown")
	}
	if a.tray != nil || a.toolDOMReady {
		t.Fatal("late DOM callback started tray after shutdown")
	}
}
