//go:build darwin

package helper

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func authorizePeer(conn *net.UnixConn, _ []byte) error {
	allowed, err := loadAllowedUID()
	if err != nil {
		return err
	}
	raw, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	var peerUID int
	var ctlErr error
	err = raw.Control(func(fd uintptr) {
		cred, e := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		if e != nil {
			ctlErr = e
			return
		}
		peerUID = int(cred.Uid)
	})
	if err != nil {
		return err
	}
	if ctlErr != nil {
		// unixgram 上常拿不到 peer cred，依赖 socket 权限 (root:staff 0660)。
		return nil
	}
	if peerUID != allowed {
		return fmt.Errorf("peer uid %d not allowed (expect %d)", peerUID, allowed)
	}
	return nil
}

func loadAllowedUID() (int, error) {
	data, err := os.ReadFile(AllowedUIDPath)
	if err != nil {
		data, err = os.ReadFile(AllowedUIDPersistPath)
		if err != nil {
			return 0, fmt.Errorf("allowed uid not configured: %w", err)
		}
	}
	uid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid allowed uid: %w", err)
	}
	return uid, nil
}
