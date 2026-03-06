//go:build darwin

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/getlantern/systray"
	"github.com/stefanos/dotward/internal/core"
	"github.com/stefanos/dotward/internal/updater"
	"github.com/stefanos/dotward/internal/version"
)

type app struct {
	cfg          core.Config
	state        *core.State
	notifier     Notifier
	extendCh     chan string
	statusItem   *systray.MenuItem
	fileItems    []*systray.MenuItem
	filePaths    []string
	fileClickCh  chan int
	wakeCh       chan struct{}
	quitItem     *systray.MenuItem
	versionItem  *systray.MenuItem
	stopRPC      func() error
	tickerStop   chan struct{}
	tickerDone   chan struct{}
	updateCh     chan updateNotification
	skipUpdateCh chan string
	updateCheck  *updater.Checker
	updatePrefs  *updater.PreferenceStore
	updateMu     sync.Mutex
	updating     bool
	shutdownOnce sync.Once
}

const maxFileMenuItems = 128

func main() {
	log.Printf("starting dotward-app %s", version.String())

	cfg, err := core.ResolveConfig()
	if err != nil {
		log.Fatalf("failed to resolve config: %v", err)
	}
	if err := core.EnsureDirs(cfg); err != nil {
		log.Fatalf("failed to initialize config dir: %v", err)
	}

	state, err := core.LoadState(cfg.StatePath)
	if err != nil {
		log.Fatalf("failed to load state: %v", err)
	}

	notifier := newNotifier()

	updatePrefs, err := updater.LoadPreferenceStore(filepath.Join(cfg.AppDir, "update-preferences.json"))
	if err != nil {
		log.Fatalf("failed to load update preferences: %v", err)
	}

	a := &app{
		cfg:          cfg,
		state:        state,
		notifier:     notifier,
		extendCh:     make(chan string, 32),
		fileItems:    make([]*systray.MenuItem, 0, maxFileMenuItems),
		filePaths:    make([]string, maxFileMenuItems),
		fileClickCh:  make(chan int, 32),
		wakeCh:       make(chan struct{}, 8),
		tickerStop:   make(chan struct{}),
		tickerDone:   make(chan struct{}),
		updateCh:     make(chan updateNotification, 8),
		skipUpdateCh: make(chan string, 8),
		updateCheck:  updater.NewChecker(),
		updatePrefs:  updatePrefs,
	}

	stopRPC, err := startRPCServer(cfg, state, notifier)
	if err != nil {
		log.Fatalf("failed to start rpc server: %v", err)
	}
	a.stopRPC = stopRPC

	systray.Run(a.onReady, a.onExit)
}

func (a *app) onReady() {
	systray.SetTitle("")
	systray.SetTooltip("Dotward is monitoring unlocked secret files")
	if err := a.notifier.Init(a.extendCh, a.updateCh, a.skipUpdateCh); err != nil {
		log.Printf("failed to initialize notifications: %v", err)
	}
	if err := initWakeMonitor(a.wakeCh); err != nil {
		log.Printf("failed to initialize wake monitor: %v", err)
	}
	a.installSignalHandler()

	a.statusItem = systray.AddMenuItem("Status: monitoring 0 files", "Current status")
	a.statusItem.Disable()
	systray.AddSeparator()
	for i := 0; i < maxFileMenuItems; i++ {
		item := systray.AddMenuItem("", "")
		item.Hide()
		idx := i
		go func() {
			for range item.ClickedCh {
				select {
				case a.fileClickCh <- idx:
				default:
				}
			}
		}()
		a.fileItems = append(a.fileItems, item)
	}
	systray.AddSeparator()
	a.versionItem = systray.AddMenuItem(fmt.Sprintf("Version: %s", version.Version), "App version")
	a.versionItem.Disable()
	a.quitItem = systray.AddMenuItem("Quit", "Stop daemon and close")
	go func() {
		for range a.quitItem.ClickedCh {
			systray.Quit()
		}
	}()

	a.checkFiles(time.Now())
	a.updateStatus()
	a.checkForUpdates()

	go a.loop()
}

func (a *app) onExit() {
	a.shutdownOnce.Do(func() {
		close(a.tickerStop)
	})
	select {
	case <-a.tickerDone:
	case <-time.After(2 * time.Second):
		log.Printf("timed out waiting for worker loop shutdown")
	}
	if a.stopRPC != nil {
		if err := a.stopRPC(); err != nil {
			log.Printf("rpc shutdown error: %v", err)
		}
	}
	a.lockAllWatchedFilesOnExit()
	if err := a.notifier.Shutdown(); err != nil {
		log.Printf("notification shutdown error: %v", err)
	}
}

func (a *app) loop() {
	defer close(a.tickerDone)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	updateTicker := time.NewTicker(24 * time.Hour)
	defer updateTicker.Stop()

	for {
		select {
		case <-a.tickerStop:
			return
		case <-ticker.C:
			a.checkFiles(time.Now())
			a.updateStatus()
		case <-updateTicker.C:
			a.checkForUpdates()
		case path := <-a.extendCh:
			if ok := a.state.Extend(path, a.cfg.DefaultTTL); ok {
				if err := a.state.Save(a.cfg.StatePath); err != nil {
					log.Printf("failed to save state after extension for %q: %v", path, err)
				}
			}
			a.updateStatus()
		case tag := <-a.skipUpdateCh:
			if err := a.updatePrefs.SetSkippedVersion(tag); err != nil {
				log.Printf("failed to persist skipped version %q: %v", tag, err)
			} else {
				log.Printf("update notifications skipped for version %q", tag)
			}
		case update := <-a.updateCh:
			a.startUpdate(update)
		case <-a.wakeCh:
			a.checkFiles(time.Now())
			a.updateStatus()
		case idx := <-a.fileClickCh:
			a.removeWatchedFileByIndex(idx)
		}
	}
}

func (a *app) checkForUpdates() {
	buildTime := version.BuildTime
	if buildTime == "" || buildTime == "unknown" {
		buildTime = version.BuildDate
	}
	if buildTime == "" || buildTime == "unknown" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	release, ok, err := a.updateCheck.Check(ctx, buildTime)
	if err != nil {
		log.Printf("failed to check for updates: %v", err)
		return
	}
	if !ok {
		return
	}

	if skipped := a.updatePrefs.SkippedVersion(); skipped != "" && skipped == release.TagName {
		log.Printf("latest release %q is skipped by user preference", release.TagName)
		return
	}

	err = a.notifier.UpdateAvailable(updateNotification{
		Version:        release.TagName,
		PublishedAt:    release.PublishedAt,
		AppDownloadURL: release.AppDownloadURL,
		CLIDownloadURL: release.CLIDownloadURL,
	})
	if err != nil {
		log.Printf("failed to send update notification for %q: %v", release.TagName, err)
	}
}

func (a *app) startUpdate(update updateNotification) {
	a.updateMu.Lock()
	if a.updating {
		a.updateMu.Unlock()
		log.Printf("update already in progress, ignoring request for %q", update.Version)
		return
	}
	a.updating = true
	a.updateMu.Unlock()

	go func() {
		defer func() {
			a.updateMu.Lock()
			a.updating = false
			a.updateMu.Unlock()
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		log.Printf("starting update to %s", update.Version)
		if err := a.applyUpdate(ctx, update); err != nil {
			log.Printf("failed to apply update %q: %v", update.Version, err)
			return
		}
		log.Printf("update %s installed successfully, quitting current app", update.Version)
		systray.Quit()
	}()
}

func (a *app) checkFiles(now time.Time) {
	changed := false
	files := a.state.Snapshot()

	for path, wf := range files {
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				a.state.StopWatching(path)
				changed = true
				continue
			}
			log.Printf("stat error for %q: %v", path, err)
			continue
		}

		if now.After(wf.ExpiresAt) {
			if err := core.SecureDelete(path); err != nil {
				log.Printf("failed to delete expired file %q: %v", path, err)
				continue
			}
			a.state.StopWatching(path)
			changed = true
			if err := a.notifier.FileDeleted(path); err != nil {
				log.Printf("failed to send delete notification for %q: %v", path, err)
			}
			continue
		}

		if !wf.Warned && now.After(wf.ExpiresAt.Add(-core.WarningWindow)) {
			if err := a.notifier.Warn(path, wf.ExpiresAt); err != nil {
				log.Printf("failed to send warning notification for %q: %v", path, err)
			} else if a.state.MarkWarned(path) {
				changed = true
			}
		}
	}

	if changed {
		if err := a.state.Save(a.cfg.StatePath); err != nil {
			log.Printf("failed to save state after checks: %v", err)
		}
	}
}

func (a *app) updateStatus() {
	if a.statusItem == nil {
		return
	}
	count := a.state.Count()
	a.statusItem.SetTitle(fmt.Sprintf("Status: monitoring %d files", count))
	systray.SetTitle("")
	systray.SetIcon(trayIconBytes(count))
	a.renderWatchedFiles()
}

func (a *app) renderWatchedFiles() {
	files := a.state.Snapshot()
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	if len(paths) > len(a.fileItems) {
		log.Printf("too many watched files for tray menu: got %d, showing first %d", len(paths), len(a.fileItems))
		paths = paths[:len(a.fileItems)]
	}

	for i := range a.fileItems {
		if i < len(paths) {
			a.filePaths[i] = paths[i]
			a.fileItems[i].SetTitle(paths[i])
			a.fileItems[i].Show()
			continue
		}
		a.filePaths[i] = ""
		a.fileItems[i].Hide()
	}
}

func (a *app) removeWatchedFileByIndex(idx int) {
	if idx < 0 || idx >= len(a.filePaths) {
		return
	}
	path := a.filePaths[idx]
	if path == "" {
		return
	}
	if err := core.SecureDelete(path); err != nil {
		log.Printf("failed to delete file selected from menu %q: %v", path, err)
		return
	}
	a.state.StopWatching(path)
	if err := a.state.Save(a.cfg.StatePath); err != nil {
		log.Printf("failed to save state after menu lock for %q: %v", path, err)
	}
	if err := a.notifier.FileDeleted(path); err != nil {
		log.Printf("failed to send delete notification for %q: %v", path, err)
	}
	a.updateStatus()
}

func (a *app) installSignalHandler() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-a.tickerStop:
			signal.Stop(ch)
		case <-ch:
			systray.Quit()
			signal.Stop(ch)
		}
	}()
}

func (a *app) lockAllWatchedFilesOnExit() {
	files := a.state.Snapshot()
	if len(files) == 0 {
		return
	}

	changed := false
	for path := range files {
		if err := core.SecureDelete(path); err != nil && !os.IsNotExist(err) {
			log.Printf("failed to delete watched file during shutdown %q: %v", path, err)
			continue
		}
		a.state.StopWatching(path)
		changed = true
	}

	if changed {
		if err := a.state.Save(a.cfg.StatePath); err != nil {
			log.Printf("failed to save state during shutdown lock: %v", err)
		}
	}
}
