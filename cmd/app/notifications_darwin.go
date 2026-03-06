//go:build darwin

package main

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework UserNotifications

#include <stdlib.h>
void DotwardInitNotifications(void);
int DotwardSendExpiryNotification(const char *path, const char *title, const char *body);
int DotwardSendUnlockedNotification(const char *path, const char *title, const char *body);
int DotwardSendDeletedNotification(const char *path, const char *title, const char *body);
int DotwardSendUpdateNotification(const char *version, const char *publishedAt, const char *appDownloadURL, const char *cliDownloadURL, const char *title, const char *body);
*/
import "C"

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"
	"unsafe"
)

var (
	extendActionMu sync.RWMutex
	extendActionCh chan<- string
	updateActionCh chan<- updateNotification
	skipActionCh   chan<- string
)

type darwinNotifier struct{}

func newNotifier() Notifier {
	return &darwinNotifier{}
}

func (n *darwinNotifier) Init(extendCh chan<- string, updateCh chan<- updateNotification, skipVersionCh chan<- string) error {
	extendActionMu.Lock()
	extendActionCh = extendCh
	updateActionCh = updateCh
	skipActionCh = skipVersionCh
	extendActionMu.Unlock()
	C.DotwardInitNotifications()
	return nil
}

func (n *darwinNotifier) Warn(path string, expiresAt time.Time) error {
	title := C.CString("Dotward Expiry Warning")
	defer C.free(unsafe.Pointer(title))

	body := C.CString(fmt.Sprintf("%s will be deleted at %s", filepath.Base(path), expiresAt.Format(time.Kitchen)))
	defer C.free(unsafe.Pointer(body))

	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	if C.DotwardSendExpiryNotification(cpath, title, body) == 0 {
		return fmt.Errorf("failed to enqueue warning notification for %q", path)
	}
	return nil
}

func (n *darwinNotifier) FileUnlocked(path string, ttl time.Duration) error {
	title := C.CString("Dotward File Unlocked")
	defer C.free(unsafe.Pointer(title))

	body := C.CString(fmt.Sprintf("%s unlocked. Expires in %s.", filepath.Base(path), ttl))
	defer C.free(unsafe.Pointer(body))

	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	if C.DotwardSendUnlockedNotification(cpath, title, body) == 0 {
		return fmt.Errorf("failed to enqueue unlocked notification for %q", path)
	}
	return nil
}

func (n *darwinNotifier) FileDeleted(path string) error {
	title := C.CString("Dotward File Deleted")
	defer C.free(unsafe.Pointer(title))

	body := C.CString(fmt.Sprintf("Deleted plaintext file %s", filepath.Base(path)))
	defer C.free(unsafe.Pointer(body))

	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	if C.DotwardSendDeletedNotification(cpath, title, body) == 0 {
		return fmt.Errorf("failed to enqueue deletion notification for %q", path)
	}
	return nil
}

func (n *darwinNotifier) Shutdown() error {
	extendActionMu.Lock()
	extendActionCh = nil
	updateActionCh = nil
	skipActionCh = nil
	extendActionMu.Unlock()
	return nil
}

func (n *darwinNotifier) UpdateAvailable(update updateNotification) error {
	title := C.CString("Dotward Update Available")
	defer C.free(unsafe.Pointer(title))

	body := C.CString(fmt.Sprintf("Version %s is available. Built on %s.", update.Version, update.PublishedAt.Format("Jan 2, 2006 15:04")))
	defer C.free(unsafe.Pointer(body))

	cVersion := C.CString(update.Version)
	defer C.free(unsafe.Pointer(cVersion))
	cPublishedAt := C.CString(update.PublishedAt.Format(time.RFC3339))
	defer C.free(unsafe.Pointer(cPublishedAt))
	cAppURL := C.CString(update.AppDownloadURL)
	defer C.free(unsafe.Pointer(cAppURL))
	cCLIURL := C.CString(update.CLIDownloadURL)
	defer C.free(unsafe.Pointer(cCLIURL))

	if C.DotwardSendUpdateNotification(cVersion, cPublishedAt, cAppURL, cCLIURL, title, body) == 0 {
		return fmt.Errorf("failed to enqueue update notification for %q", update.Version)
	}
	return nil
}

// HandleExtendAction bridges the native Extend action callback into Go.
//
//export HandleExtendAction
func HandleExtendAction(path *C.char) {
	defer C.free(unsafe.Pointer(path))
	if path == nil {
		return
	}
	p := C.GoString(path)
	extendActionMu.RLock()
	ch := extendActionCh
	extendActionMu.RUnlock()
	if ch == nil || p == "" {
		return
	}
	select {
	case ch <- p:
	default:
	}
}

// HandleUpdateAction bridges the native Update action callback into Go.
//
//export HandleUpdateAction
func HandleUpdateAction(versionTag *C.char, publishedAt *C.char, appDownloadURL *C.char, cliDownloadURL *C.char) {
	defer C.free(unsafe.Pointer(versionTag))
	defer C.free(unsafe.Pointer(publishedAt))
	defer C.free(unsafe.Pointer(appDownloadURL))
	defer C.free(unsafe.Pointer(cliDownloadURL))

	if versionTag == nil || publishedAt == nil || appDownloadURL == nil || cliDownloadURL == nil {
		return
	}

	tag := C.GoString(versionTag)
	publishedRaw := C.GoString(publishedAt)
	appURL := C.GoString(appDownloadURL)
	cliURL := C.GoString(cliDownloadURL)
	if tag == "" || publishedRaw == "" || appURL == "" || cliURL == "" {
		return
	}

	published, err := time.Parse(time.RFC3339, publishedRaw)
	if err != nil {
		return
	}

	extendActionMu.RLock()
	ch := updateActionCh
	extendActionMu.RUnlock()
	if ch == nil {
		return
	}

	select {
	case ch <- updateNotification{
		Version:        tag,
		PublishedAt:    published,
		AppDownloadURL: appURL,
		CLIDownloadURL: cliURL,
	}:
	default:
	}
}

// HandleSkipVersionAction bridges the native Skip Version action callback into Go.
//
//export HandleSkipVersionAction
func HandleSkipVersionAction(versionTag *C.char) {
	defer C.free(unsafe.Pointer(versionTag))
	if versionTag == nil {
		return
	}

	tag := C.GoString(versionTag)
	if tag == "" {
		return
	}

	extendActionMu.RLock()
	ch := skipActionCh
	extendActionMu.RUnlock()
	if ch == nil {
		return
	}

	select {
	case ch <- tag:
	default:
	}
}
