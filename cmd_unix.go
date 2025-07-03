//go:build !windows
// +build !windows

package supervisor

import (
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

func configureSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}
}

func configureExecutingUser(cmd *exec.Cmd, username string) {
	if username != "" {
		currentUser, _ := user.Current()

		if currentUser.Username != username {
			executingUser, _ := user.Lookup(username)

			uid, _ := strconv.Atoi(executingUser.Uid)
			gid, _ := strconv.Atoi(executingUser.Gid)

			cmd.SysProcAttr.Credential = &syscall.Credential{
				Uid: uint32(uid),
				Gid: uint32(gid),
			}
		}
	}
}

func signalNameToSignal(signalName string) os.Signal {
	resolvedSignal := unix.SignalNum(signalName)
	if resolvedSignal != 0 {
		return resolvedSignal
	}

	return os.Interrupt
}
