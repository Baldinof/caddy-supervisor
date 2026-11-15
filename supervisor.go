package supervisor

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
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
	Options Options

	// logger can be replaced while running: with "on_reload keep" a supervisor outlives the config that created
	// it, and the loggers of a replaced config must not be used anymore.
	logger atomic.Pointer[zap.Logger]

	mu sync.Mutex
	// cmd is the running process, nil while none is running
	cmd *exec.Cmd
	// stopped is set once by Stop(): a stopped supervisor never starts a process again
	stopped bool
	// stopping is closed by Stop() to interrupt the delay before a restart
	stopping chan struct{}
	// exited is closed when the supervising loop has ended; nil while Run() was not called
	exited chan struct{}
}

func newSupervisor(options Options, logger *zap.Logger) *Supervisor {
	s := &Supervisor{Options: options, stopping: make(chan struct{})}
	s.setLogger(logger)

	return s
}

func (s *Supervisor) setLogger(logger *zap.Logger) {
	s.logger.Store(logger)
}

func (s *Supervisor) log() *zap.Logger {
	return s.logger.Load()
}

// Run a process and supervise
func (s *Supervisor) Run() {
	exited := make(chan struct{})

	s.mu.Lock()
	if s.exited != nil {
		// already supervising
		s.mu.Unlock()
		return
	}
	s.exited = exited
	s.mu.Unlock()

	defer close(exited)

	restartDelay := minRestartDelay

	for {
		cmd := exec.Command(s.Options.Command, s.Options.Args...)
		configureSysProcAttr(cmd)
		cmd.Env = append(os.Environ(), s.Options.Env...)

		configureExecutingUser(cmd, s.Options.User)

		if s.Options.Dir != "" {
			cmd.Dir = s.Options.Dir
		}

		var afterRun []func()

		if outputFile, closeFile, err := getOutputFile(s.Options.RedirectStdout); err == nil {
			cmd.Stdout = outputFile
			afterRun = append(afterRun, closeFile)
		} else {
			s.log().Error("cannot setup stdout redirection", zap.Error(err), zap.String("target", s.Options.RedirectStdout.string()))
		}

		if outputFile, closeFile, err := getOutputFile(s.Options.RedirectStderr); err == nil {
			cmd.Stderr = outputFile
			afterRun = append(afterRun, closeFile)
		} else {
			s.log().Error("cannot setup stderr redirection", zap.Error(err), zap.String("target", s.Options.RedirectStderr.string()))
		}

		// Starting and stopping exclude each other: Stop() either prevents the start or finds the started process.
		// Without this, a Stop() arriving before the process was started left it running forever.
		s.mu.Lock()
		if s.stopped {
			s.mu.Unlock()
			for _, fn := range afterRun {
				fn()
			}
			return
		}

		start := time.Now()
		err := cmd.Start()

		failedAtStart := err != nil
		if !failedAtStart {
			s.cmd = cmd
		}
		s.mu.Unlock()

		if failedAtStart {
			s.log().Error("failed to start process", zap.Error(err))
		} else {
			s.log().Info("process started", zap.Int("pid", cmd.Process.Pid))

			err = cmd.Wait()

			s.mu.Lock()
			s.cmd = nil
			s.mu.Unlock()
		}

		duration := time.Since(start)

		for _, fn := range afterRun {
			fn()
		}

		if err != nil {
			if !failedAtStart {
				s.log().Error("process exited with error", zap.Error(err), zap.Duration("duration", duration))
			}
		} else {
			s.log().Info("process exited", zap.Duration("duration", duration))
		}

		if s.isStopped() {
			return
		}

		switch s.Options.RestartPolicy {
		case RestartOnFailure:
			if err == nil {
				return
			}
		case RestartNever:
			return
		}

		if restartDelay > minRestartDelay && (err == nil || duration > durationToResetRestartDelay) {
			s.log().Info("resetting restart delay", zap.Duration("delay", minRestartDelay))
			restartDelay = minRestartDelay
		}

		if err != nil {
			s.log().Info("process will restart", zap.Duration("wait_delay", restartDelay))

			select {
			case <-time.After(restartDelay):
			case <-s.stopping:
				return
			}

			restartDelay = increaseRestartDelay(restartDelay)
		}
	}
}

// Stop the supervised process. It returns when the process has exited, so that a successor (another process using
// the same port, the next config generation) does not overlap with it.
func (s *Supervisor) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	close(s.stopping)
	cmd := s.cmd
	exited := s.exited
	s.mu.Unlock()

	if exited == nil {
		// Run() was not called yet; if it is called later it returns without starting anything
		return
	}

	if cmd != nil {
		s.log().Debug("asking the process to stop")

		// a signal on unix-like systems, Ctrl+Break to the process group of the command on Windows
		if err := requestStop(cmd, s.Options.StopSignal); err == nil {
			select {
			case <-exited:
				return
			case <-time.After(s.Options.TerminationGracePeriod):
				s.log().
					With(zap.Duration("grace_period", s.Options.TerminationGracePeriod)).
					Info("termination grace period exceeded, killing")
			}
		} else {
			// e.g. on Windows when Caddy runs without a console
			s.log().
				With(zap.Error(err)).
				Info("cannot ask the process to stop, killing")
		}

		// an error means the process is already gone
		_ = cmd.Process.Kill()
	}

	<-exited
}

// Destruct implements caddy.Destructor: with "on_reload keep" the process is stopped when no config uses it anymore.
func (s *Supervisor) Destruct() error {
	s.Stop()

	return nil
}

func (s *Supervisor) isStopped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.stopped
}

func (s *Supervisor) isRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.cmd != nil
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
