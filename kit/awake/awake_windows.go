package awake

import (
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

var queryUnbiasedInterruptTime = syscall.NewLazyDLL("kernel32.dll").NewProc("QueryUnbiasedInterruptTime")

// awakeTime is Windows' unbiased interrupt time, in units of 100
// nanoseconds, which does not count time asleep or hibernating.
func awakeTime() time.Duration {
	var t uint64
	if r, _, _ := queryUnbiasedInterruptTime.Call(uintptr(unsafe.Pointer(&t))); r == 0 {
		return goAwakeTime()
	}
	return time.Duration(t) * 100
}

// Windows: SetThreadExecutionState, which holds while the thread that
// called it lives, so it is called on a thread kept for it until release.
var setThreadExecutionState = syscall.NewLazyDLL("kernel32.dll").NewProc("SetThreadExecutionState")

const (
	esContinuous     = 0x80000000
	esSystemRequired = 0x00000001
)

func hold(string) (func(), error) {
	if err := setThreadExecutionState.Find(); err != nil {
		return nil, err
	}
	started, release := make(chan error), make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if r, _, err := setThreadExecutionState.Call(esContinuous | esSystemRequired); r == 0 {
			started <- err
			return
		}
		started <- nil
		<-release
		setThreadExecutionState.Call(esContinuous)
	}()
	if err := <-started; err != nil {
		return nil, err
	}
	return func() { close(release) }, nil
}
