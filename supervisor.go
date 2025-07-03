package supervisor

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"go.uber.org/zap"
)

var emptyFunc = func() {}

const (
	minRestartDelay             = time.Duration(0)
	maxRestartDelay             = 5 * time.Minute
	durationToResetRestartDelay = 10 * time.Minute
)

// Supervisor provides functionality to start and supervise a background process
type Supervisor struct {
	Options     Options
	cmd         *exec.Cmd
	keepRunning bool
	logger      *zap.Logger
}

// Run a process and supervise
func (s *Supervisor) Run() {
	s.keepRunning = true

	restartDelay := minRestartDelay

	for s.keepRunning {
		s.cmd = exec.Command(s.Options.Command, s.Options.Args...)
		configureSysProcAttr(s.cmd)
		s.cmd.Env = append(os.Environ(), s.Options.Env...)

		configureExecutingUser(s.cmd, s.Options.User)

		if s.Options.Dir != "" {
			s.cmd.Dir = s.Options.Dir
		}

		var afterRun []func()

		if outputFile, closeFile, err := getOutputFile(s.Options.RedirectStdout); err == nil {
			s.cmd.Stdout = outputFile
			afterRun = append(afterRun, closeFile)
		} else {
			s.logger.Error("cannot setup stdout redirection", zap.Error(err), zap.String("target", s.Options.RedirectStdout.string()))
		}

		if outputFile, closeFile, err := getOutputFile(s.Options.RedirectStderr); err == nil {
			s.cmd.Stderr = outputFile
			afterRun = append(afterRun, closeFile)
		} else {
			s.logger.Error("cannot setup stderr redirection", zap.Error(err), zap.String("target", s.Options.RedirectStderr.string()))
		}

		start := time.Now()
		err := s.cmd.Start()

		failedAtStart := false
		if err != nil {
			s.logger.Error("failed to start process", zap.Error(err))
			failedAtStart = true
		} else {
			s.logger.Info("process started", zap.Int("pid", s.cmd.Process.Pid))

			err = s.cmd.Wait()
		}

		duration := time.Since(start)

		for _, fn := range afterRun {
			fn()
		}

		if err != nil {
			if !failedAtStart {
				s.logger.Error("process exited with error", zap.Error(err), zap.Duration("duration", duration))
			}
		} else {
			s.logger.Info("process exited", zap.Duration("duration", duration))
		}

		if !s.keepRunning {
			break
		}

		switch s.Options.RestartPolicy {
		case RestartAlways:
			break
		case RestartOnFailure:
			if err == nil {
				s.keepRunning = false
			}
			break
		case RestartNever:
			s.keepRunning = false
			break
		}

		if s.keepRunning {
			if restartDelay > minRestartDelay && (err == nil || duration > durationToResetRestartDelay) {
				s.logger.Info("resetting restart delay", zap.Duration("delay", minRestartDelay))
				restartDelay = minRestartDelay
			}

			if err != nil {
				s.logger.Info("process will restart", zap.Duration("wait_delay", restartDelay))
				time.Sleep(restartDelay)
				restartDelay = increaseRestartDelay(restartDelay)
			}
		}
	}
}

// Stop the supervised process
func (s *Supervisor) Stop() {
	s.keepRunning = false

	if cmdIsRunning(s.cmd) {
		s.logger.Debug("sending 'interrupt signal to gracefully stop the process")

		err := s.cmd.Process.Signal(s.Options.StopSignal)
		if err == nil {
			go func() {
				time.Sleep(s.Options.TerminationGracePeriod)
				if cmdIsRunning(s.cmd) {
					s.logger.
						With(zap.Duration("grace_period", s.Options.TerminationGracePeriod)).
						Info("termination grace period exceeded, killing")

					s.cmd.Process.Kill()
				}
			}()

			s.cmd.Wait()
		} else {
			s.logger.
				With(zap.Error(err)).
				Info("error while sending 'interup' signal, killing")

			s.cmd.Process.Kill()
		}
	}
}

func cmdIsRunning(cmd *exec.Cmd) bool {
	return cmd != nil && cmd.Process != nil && (cmd.ProcessState == nil || !cmd.ProcessState.Exited())
}

func getOutputFile(target OutputTarget) (*os.File, func(), error) {
	switch target.Type {
	case OutputTypeNull:
		return nil, emptyFunc, nil
	case OutputTypeStdout:
		return os.Stdout, emptyFunc, nil
	case OutputTypeStderr:
		return os.Stderr, emptyFunc, nil
	case OutputTypeFile:
		if target.File == "" {
			return nil, nil, errors.New("invalid output target, file should be defined")
		}

		outFile, err := os.OpenFile(target.File, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
		if err != nil {
			return nil, nil, err
		}

		return outFile, func() {
			outFile.Close()
		}, nil
	}

	return nil, emptyFunc, fmt.Errorf("unsupported output target type '%s', allowed: null, stderr, stdout, file", target.Type)
}

func increaseRestartDelay(restartDelay time.Duration) time.Duration {
	if restartDelay == 0 {
		return 1 * time.Second
	}

	restartDelay = restartDelay * 2

	if restartDelay > maxRestartDelay {
		restartDelay = maxRestartDelay
	}

	return restartDelay
}
