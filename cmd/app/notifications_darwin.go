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
)

type darwinNotifier struct{}

func newNotifier() Notifier {
	return &darwinNotifier{}
}

func (n *darwinNotifier) Init(extendCh chan<- string) error {
	extendActionMu.Lock()
	extendActionCh = extendCh
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
	extendActionMu.Unlock()
	return nil
}

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
