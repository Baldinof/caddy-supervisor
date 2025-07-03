package supervisor

import (
	"os/exec"

	"os"
)

func configureSysProcAttr(cmd *exec.Cmd) {

}

func configureExecutingUser(cmd *exec.Cmd, username string) {

}

func signalNameToSignal(_ string) os.Signal {
	return os.Interrupt
}
