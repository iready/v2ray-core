//go:build !darwin && !windows

package tunctl

func preferredAutoBindInterface() (string, error) {
	return "", errNoDefaultRouteIface
}
