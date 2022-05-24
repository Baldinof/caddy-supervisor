package supervisor

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/caddyserver/caddy/v2/caddyconfig"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
)

func init() {
	httpcaddyfile.RegisterGlobalOption("supervisor", parseSupervisor)
}

// parseSupervisor configures the "exec" global option from Caddyfile.
// Syntax:
//  supervisor {
//		php-fpm -d /etc/php-fpm/config.ini {
//	    	dir /home/user
//			redirect_stdout stdout
//			redirect_stderr stderr
//			restart_policy always
//			env MAX_CHILDREN 5
//	  	}
//  }
func parseSupervisor(d *caddyfile.Dispenser, _ interface{}) (interface{}, error) {
	app := new(App)

	// consume the option name
	if !d.Next() {
		return nil, d.ArgErr()
	}

	// handle the block, can have more than one command defined
	for d.NextBlock(0) {
		def := Definition{}

		def.Command = append([]string{d.Val()}, d.RemainingArgs()...)

		if len(def.Command) == 0 {
			return nil, d.ArgErr()
		}

		// handle any options
		for d.NextBlock(1) {
			switch d.Val() {
			case "dir":
				if !d.Args(&def.Dir) {
					return nil, d.ArgErr()
				}
			case "redirect_stdout":
				target, err := parseOutpoutRedirect(d)

				if err != nil {
					return nil, err
				}

				def.RedirectStdout = target
			case "redirect_stderr":
				target, err := parseOutpoutRedirect(d)

				if err != nil {
					return nil, err
				}

				def.RedirectStderr = target
			case "restart_policy":
				var p string

				if !d.Args(&p) {
					return nil, d.ArgErr()
				}

				if p != string(RestartAlways) && p != string(RestartNever) && p != string(RestartOnFailure) {
					return nil, d.Errf("'restart_policy' should be either '%s', '%s', or '%s': '%s' given", RestartAlways, RestartNever, RestartOnFailure, p)
				}

				def.RestartPolicy = RestartPolicy(p)
			case "termination_grace_period":
				if !d.Args(&def.TerminationGracePeriod) {
					return nil, d.ArgErr()
				}

				if _, err := time.ParseDuration(def.TerminationGracePeriod); err != nil {
					return nil, d.Errf("cannot parse 'termination_grace_period' into time.Duration, '%s' given", def.TerminationGracePeriod)
				}
			case "replicas":
				var replicas string

				if !d.Args(&replicas) {
					return nil, d.ArgErr()
				}

				r, err := strconv.Atoi(replicas)

				if err != nil {
					return nil, d.Errf("'replicas' should be a positive integer, '%s' given", replicas)
				}

				if r < 0 {
					return nil, d.Errf("'replicas' should be a positive integer, '%s' given", replicas)
				}

				def.Replicas = &r
			case "env":
				var envKey, envValue string

				if !d.Args(&envKey, &envValue) {
					return nil, d.ArgErr()
				}

				remaining := d.RemainingArgs()

				if len(remaining) != 0 {
					envValue = fmt.Sprintf("%s %s", envValue, strings.Join(remaining, " "))
				}

				if def.Env == nil {
					def.Env = map[string]string{}
				}

				def.Env[envKey] = envValue
			case "user":
				var user string

				if !d.Args(&user) {
					return nil, d.ArgErr()
				}

				def.User = user
			}
		}

		app.Supervise = append(app.Supervise, def)
	}

	// tell Caddyfile adapter that this is the JSON for an app
	return httpcaddyfile.App{
		Name:  "supervisor",
		Value: caddyconfig.JSON(app, nil),
	}, nil
}

func parseOutpoutRedirect(d *caddyfile.Dispenser) (*OutputTarget, error) {
	if !d.NextArg() {
		return nil, d.ArgErr()
	}

	switch d.Val() {
	case OutputTypeNull, OutputTypeStdout, OutputTypeStderr:
		return &OutputTarget{Type: d.Val()}, nil
	case OutputTypeFile:
		var file string
		if !d.Args(&file) {
			return nil, d.ArgErr()
		}

		return &OutputTarget{Type: OutputTypeFile, File: file}, nil
	default:
		return nil, d.Errf("target should be either '%s', '%s', '%s',  or '%s': '%s' given", OutputTypeNull, OutputTypeStdout, OutputTypeStderr, OutputTypeFile, d.Val())
	}
}
