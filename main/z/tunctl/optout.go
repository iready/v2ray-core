package tunctl

import "sync"

var userOptOut struct {
	mu       sync.Mutex
	disabled bool
}

// SetUserDisabled records whether the user turned off platform TUN via localadmin.
// When true, PrepareConfigs skips automatic enable until cleared.
func SetUserDisabled(disabled bool) {
	userOptOut.mu.Lock()
	userOptOut.disabled = disabled
	userOptOut.mu.Unlock()
}

func userOptedOut() bool {
	userOptOut.mu.Lock()
	defer userOptOut.mu.Unlock()
	return userOptOut.disabled
}
