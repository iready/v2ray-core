//go:build darwin

package helper

import (
	"fmt"
	"net"
	"os"
	"time"
)

func listenFDPass(sockPath string) (*net.UnixListener, error) {
	_ = os.Remove(sockPath)
	addr, err := net.ResolveUnixAddr("unix", sockPath)
	if err != nil {
		return nil, err
	}
	ln, err := net.ListenUnix("unix", addr)
	if err != nil {
		return nil, err
	}
	const staffGID = 20
	_ = os.Chown(sockPath, 0, staffGID)
	_ = os.Chmod(sockPath, 0o660)
	return ln, nil
}

// serveFDOnce 在已监听的 unix stream 上向首个连接方传递 fd。
func serveFDOnce(ln *net.UnixListener, sockPath string, fd int) error {
	defer os.Remove(sockPath)
	defer ln.Close()

	errCh := make(chan error, 1)
	go func() {
		_ = ln.SetDeadline(time.Now().Add(10 * time.Second))
		c, err := ln.AcceptUnix()
		if err != nil {
			errCh <- err
			return
		}
		defer c.Close()
		_, _, err = c.WriteMsgUnix([]byte("OK\n"), unixRights(fd), nil)
		errCh <- err
	}()

	select {
	case err := <-errCh:
		return err
	case <-time.After(12 * time.Second):
		return fmt.Errorf("fd pass timeout")
	}
}
