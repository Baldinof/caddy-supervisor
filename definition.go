package supervisor

import (
	"errors"
	"fmt"
	"os/user"
	"strings"
	"time"

	"go.uber.org/zap"
)

// Definition is the configuration for process to supervise
type Definition struct {
	// Command to start and supervise. First item is the program to start, others are arguments.
	// Supports template.
	Command []string `json:"command"`
	// Replicas control how many instances of Command should run.
	Replicas *int `json:"replicas,omitempty"`
	// Dir defines the working directory the command should be executed in.
	// Supports template.
	// Default: current working dir
	Dir string `json:"dir,omitempty"`
	// Env declares environment variables that should be passed to command.
	// Supports template.
	Env map[string]string `json:"env,omitempty"`
	// RedirectStdout is the file where Command stdout is written. Use "stdout" to redirect to caddy stdout.
	RedirectStdout *OutputTarget `json:"redirect_stdout,omitempty"`
	// RedirectStderr is the file where Command stderr is written. Use "stderr" to redirect to caddy stderr.
	RedirectStderr *OutputTarget `json:"redirect_stderr,omitempty"`
	// RestartPolicy define under which conditions the command should be restarted after exit.
	// Valid values:
	//  - **never**: do not restart the command
	//  - **on_failure**: restart if exit code is not 0
	//  - **always**: always restart
	RestartPolicy RestartPolicy `json:"restart_policy,omitempty"`
	// TerminationGracePeriod defines the amount of time to wait for Command graceful termination before killing it. Ex: 10s
	TerminationGracePeriod string `json:"termination_grace_period,omitempty"`
	// User defines the user which executes the Command.
	// Default: current user
	User string `json:"user,omitempty"`
}

type OutputTarget struct {
	// Type is how the output should be redirected
	// Valid values:
	//   - **null**: discard outputs
	//   - **stdout**: redirect output to caddy process stdout
	//   - **stderr**: redirect output to caddy process stderr
	//   - **file**: redirect output to a file, if selected File field is required
	Type string `json:"type,omitempty"`
	// File is the file where outputs should be written. This is used only when Type is "file".
	File string `json:"file,omitempty"`
}

const (
	OutputTypeStdout = "stdout"
	OutputTypeStderr = "stderr"
	OutputTypeNull   = "null"
	OutputTypeFile   = "file"
)

// ToSupervisors creates supervisors from the Definition (one per replica) and applies templates where needed
func (d Definition) ToSupervisors(logger *zap.Logger) ([]*Supervisor, error) {
	var supervisors []*Supervisor

	opts := &Options{
		Command:       d.Command[0],
		Args:          d.Command[1:],
		Dir:           d.Dir,
		Env:           d.envToCmdArg(),
		RestartPolicy: d.RestartPolicy,
		User:          d.User,
	}

	if d.User != "" {
		if _, err := user.Lookup(d.User); err != nil {
			return supervisors, err
		}
	}

	var replicas int

	if d.Replicas == nil {
		replicas = 1
	} else {
		replicas = *d.Replicas
	}

	if opts.RestartPolicy == "" {
		opts.RestartPolicy = RestartAlways
	}

	if d.RedirectStdout == nil {
		opts.RedirectStdout = OutputTarget{Type: OutputTypeStdout}
	} else {
		if err := validateOutputTarget(*d.RedirectStdout); err != nil {
			return supervisors, err
		}
		opts.RedirectStdout = *d.RedirectStdout
	}

	if d.RedirectStderr == nil {
		opts.RedirectStderr = OutputTarget{Type: OutputTypeStderr}
	} else {
		if err := validateOutputTarget(*d.RedirectStderr); err != nil {
			return supervisors, err
		}
		opts.RedirectStderr = *d.RedirectStderr
	}

	if d.TerminationGracePeriod == "" {
		opts.TerminationGracePeriod = 10 * time.Second
	} else {
		var err error
		opts.TerminationGracePeriod, err = time.ParseDuration(d.TerminationGracePeriod)

		if err != nil {
			return supervisors, fmt.Errorf("cannot parse termination grace period of supervisor '%s'", strings.Join(d.Command, " "))
		}
	}

	for i := 0; i < replicas; i++ {
		opts.Replica = i

		templatedOpts, err := opts.processTemplates()

		if err != nil {
			return supervisors, err
		}

		supervisor := &Supervisor{
			Options: templatedOpts,
			logger: logger.
				With(zap.Strings("command", d.Command)).
				With(zap.Int("replica", templatedOpts.Replica)),
		}

		supervisors = append(supervisors, supervisor)
	}

	return supervisors, nil
}

func (d Definition) envToCmdArg() []string {
	env := make([]string, len(d.Env))
	i := 0

	for key, value := range d.Env {
		env[i] = fmt.Sprintf("%s=%s", key, value)
		i++
	}

	return env
}

func validateOutputTarget(target OutputTarget) error {
	switch target.Type {
	case OutputTypeNull, OutputTypeStdout, OutputTypeStderr:
		return nil
	case OutputTypeFile:
		if target.File == "" {
			return errors.New("invalid output target, file should be defined")
		}

		return nil
	}

	return fmt.Errorf("unsupported output target type '%s', allowed: null, stderr, stdout, file", target.Type)
}

func (t OutputTarget) string() string {
	switch t.Type {
	case OutputTypeNull, OutputTypeStdout, OutputTypeStderr:
		return t.Type
	case OutputTypeFile:
		return fmt.Sprintf("file(%s)", t.File)
	default:
		return "unknown"
	}
}
