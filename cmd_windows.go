package supervisor

import (
	"os/exec"
	"syscall"

	"os"

	"golang.org/x/sys/windows"
)

// configureSysProcAttr starts the command in its own process group. Windows has no signals; the only way to ask a
// console process to stop is a console control event, and CTRL_BREAK can be sent to a single process group.
// Without the own group the event would also hit Caddy itself.
func configureSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

func configureExecutingUser(cmd *exec.Cmd, username string) {

}

func signalNameToSignal(_ string) os.Signal {
	return os.Interrupt
}

// requestStop asks the process to stop by sending CTRL_BREAK to its process group. The process sees it like a
// Ctrl+Break in its console window: programs with a handler shut down cleanly, the others are ended by the default
// handler. It fails if Caddy has no console (the caller then kills the process), e.g. when started detached.
func requestStop(cmd *exec.Cmd, _ os.Signal) error {
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(cmd.Process.Pid))
}
