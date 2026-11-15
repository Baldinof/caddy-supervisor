package supervisor

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/Masterminds/sprig/v3"
)

// Options exposes settings to create a process supervisor
type Options struct {
	Command                string
	Replica                int
	Args                   []string
	Dir                    string
	Env                    []string
	RedirectStdout         OutputTarget
	RedirectStderr         OutputTarget
	RestartPolicy          RestartPolicy
	TerminationGracePeriod time.Duration
	User                   string
	StopSignal             os.Signal
	OnReload               OnReload
}

// OnReload determines what happens to a supervised process when the config is reloaded
type OnReload string

const (
	// OnReloadRestart stops the process and starts it again with every config reload
	OnReloadRestart = OnReload("restart")
	// OnReloadKeep leaves the process running across config reloads as long as its definition is unchanged
	OnReloadKeep = OnReload("keep")
)

// key identifies the process an Options value describes. Two configs with the same key mean the same process.
func (options Options) key() string {
	return fmt.Sprintf("%q", []interface{}{
		options.Command, options.Args, options.Replica, options.Dir, options.Env,
		options.RedirectStdout.string(), options.RedirectStderr.string(), string(options.RestartPolicy),
		options.TerminationGracePeriod.String(), options.User, fmt.Sprint(options.StopSignal),
	})
}

// RestartPolicy determines when a supervised process should be restarted
type RestartPolicy string

const (
	// RestartNever indicates to never restart the process
	RestartNever = RestartPolicy("never")
	// RestartOnFailure indicates to only restart the process after failures
	RestartOnFailure = RestartPolicy("on_failure")
	// RestartAlways indicates to always restart the process
	RestartAlways = RestartPolicy("always")
)

func (options Options) processTemplates() (Options, error) {
	result := Options{}
	var err error
	var tplErrors []string

	handleError := func(option string, e error) {
		if e != nil {
			tplErrors = append(tplErrors, fmt.Sprintf("%s: %s", option, e))
		}
	}

	result.Command, err = processTemplates(options.Command, options)
	handleError("command", err)

	result.Args = make([]string, len(options.Args))
	for i, arg := range options.Args {
		result.Args[i], err = processTemplates(arg, options)
		handleError(fmt.Sprintf("args[%d]", i), err)
	}

	result.Dir, err = processTemplates(options.Dir, options)
	handleError("dir", err)

	result.Env = make([]string, len(options.Env))
	for i, env := range options.Env {
		result.Env[i], err = processTemplates(env, options)
		handleError(fmt.Sprintf("env[%d]", i), err)
	}

	result.RedirectStdout = options.RedirectStdout
	result.RedirectStderr = options.RedirectStderr
	result.RestartPolicy = options.RestartPolicy
	result.TerminationGracePeriod = options.TerminationGracePeriod
	result.StopSignal = options.StopSignal
	result.Replica = options.Replica
	result.OnReload = options.OnReload

	if len(tplErrors) > 0 {
		return result, errors.New("failed to process templates: \n" + strings.Join(tplErrors, "\n"))
	}

	return result, nil
}

func processTemplates(text string, data interface{}) (string, error) {
	tmpl, err := template.New("supervisor").Funcs(sprig.TxtFuncMap()).Parse(text)
	if err != nil {
		return "", err
	}

	var writer bytes.Buffer
	err = tmpl.Execute(&writer, data)

	if err != nil {
		return "", err
	}

	return writer.String(), nil
}
