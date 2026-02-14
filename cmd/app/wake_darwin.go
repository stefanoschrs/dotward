//go:build darwin

package main

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework AppKit

void DotwardRegisterWakeObserver(void);
*/
import "C"

import "sync"

var (
	wakeMu       sync.RWMutex
	wakeSignalCh chan<- struct{}
)

func initWakeMonitor(ch chan<- struct{}) error {
	wakeMu.Lock()
	wakeSignalCh = ch
	wakeMu.Unlock()
	C.DotwardRegisterWakeObserver()
	return nil
}

//export HandleSystemWake
func HandleSystemWake() {
	wakeMu.RLock()
	ch := wakeSignalCh
	wakeMu.RUnlock()
	if ch == nil {
		return
	}
	select {
	case ch <- struct{}{}:
	default:
	}
}
