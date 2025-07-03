package supervisor

import (
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/stretchr/testify/assert"
)

func Test_parseSupervisor(t *testing.T) {
	tests := []struct {
		name           string
		givenCaddyfile string
		expectJson     string
		expectError    string
	}{
		{
			name: "no block no args",
			givenCaddyfile: `
				supervisor {
				  php-fpm
				}
			`,
			expectJson: `
				{
					"supervise":[
						{
							"command":["php-fpm"]
						}
					]
				}
			`,
		},
		{
			name: "no block with args",
			givenCaddyfile: `
				supervisor {
				  php-fpm --fpm-config=fpm-8.0.2.ini
				}
			`,
			expectJson: `
				{
					"supervise":[
						{
							"command":["php-fpm","--fpm-config=fpm-8.0.2.ini"]
						}
					]
				}
			`,
		},
		{
			name: "multiple no block",
			givenCaddyfile: `
				supervisor {
				  php-fpm --fpm-config=fpm-8.0.2.ini
				  node worker.js
				}
			`,
			expectJson: `
				{
					"supervise":[
						{
							"command":["php-fpm","--fpm-config=fpm-8.0.2.ini"]
						},
						{
							"command":["node","worker.js"]
						}
					]
				}
			`,
		},
		{
			name: "env vars",
			givenCaddyfile: `
				supervisor {
				  php-fpm --fpm-config=fpm-8.0.2.ini {
					env HELLO_WORLD world
					env FOO "bar baz"
				    env BAR foo baz
				  }
				}
			`,
			expectJson: `
				{
					"supervise": [
						{
							"command":["php-fpm","--fpm-config=fpm-8.0.2.ini"],
							"env": {
								"HELLO_WORLD":"world", 
								"FOO": "bar baz",
								"BAR": "foo baz"
							}
						}
					]
				}
			`,
		},
		{
			name: "env vars - error",
			givenCaddyfile: `
				supervisor {
				  php-fpm --fpm-config=fpm-8.0.2.ini {
					env HELLO_WORLD
				  }
				}
			`,
			expectError: "wrong argument count or unexpected line ending after 'HELLO_WORLD', at Testfile:4",
		},
		{
			name: "output redirections to file",
			givenCaddyfile: `
				supervisor {
				  php-fpm --fpm-config=fpm-8.0.2.ini {
					redirect_stdout file fpm-stdout.log
					redirect_stderr file fpm-stderr.log
				  }
				}
			`,
			expectJson: `
				{
					"supervise": [
						{
							"command":["php-fpm","--fpm-config=fpm-8.0.2.ini"],
							"redirect_stdout": {"type": "file", "file": "fpm-stdout.log"},
							"redirect_stderr": {"type": "file", "file": "fpm-stderr.log"}
						}
					]
				}
			`,
		},
		{
			name: "output redirections to std",
			givenCaddyfile: `
				supervisor {
				  php-fpm --fpm-config=fpm-8.0.2.ini {
					redirect_stdout stdout
					redirect_stderr stderr
				  }
				}
			`,
			expectJson: `
				{
					"supervise": [
						{
							"command":["php-fpm","--fpm-config=fpm-8.0.2.ini"],
							"redirect_stdout": {"type": "stdout"},
							"redirect_stderr": {"type": "stderr"}
						}
					]
				}
			`,
		},
		{
			name: "output redirections to std",
			givenCaddyfile: `
				supervisor {
				  php-fpm --fpm-config=fpm-8.0.2.ini {
					redirect_stdout null
				  }
				}
			`,
			expectJson: `
				{
					"supervise": [
						{
							"command":["php-fpm","--fpm-config=fpm-8.0.2.ini"],
							"redirect_stdout": {"type": "null"}
						}
					]
				}
			`,
		},
		{
			name: "redirect_stdout error",
			givenCaddyfile: `
				supervisor {
				  php-fpm --fpm-config=fpm-8.0.2.ini {
					redirect_stdout
				  }
				}
			`,
			expectError: "wrong argument count or unexpected line ending after 'redirect_stdout', at Testfile:4",
		},
		{
			name: "redirect_stderr error",
			givenCaddyfile: `
				supervisor {
				  php-fpm --fpm-config=fpm-8.0.2.ini {
					redirect_stderr
				  }
				}
			`,
			expectError: "wrong argument count or unexpected line ending after 'redirect_stderr', at Testfile:4",
		},
		{
			name: "user is provided",
			givenCaddyfile: `
				supervisor {
				  php-fpm --fpm-config=fpm-8.0.2.ini {
					user www-data
				  }
				}
			`,
			expectJson: `
				{
					"supervise": [
						{
							"command":["php-fpm","--fpm-config=fpm-8.0.2.ini"],
							"user": "www-data"
						}
					]
				}
			`,
		},
		{
			name: "replicas",
			givenCaddyfile: `
				supervisor {
				  php-fpm --fpm-config=fpm-8.0.2.ini {
					replicas 3
				  }
				}
			`,
			expectJson: `
				{
					"supervise": [
						{
							"command":["php-fpm","--fpm-config=fpm-8.0.2.ini"],
							"replicas": 3
						}
					]
				}
			`,
		},
		{
			name: "replicas zero",
			givenCaddyfile: `
				supervisor {
				  php-fpm --fpm-config=fpm-8.0.2.ini {
					replicas 0
				  }
				}
			`,
			expectJson: `
				{
					"supervise": [
						{
							"command":["php-fpm","--fpm-config=fpm-8.0.2.ini"],
							"replicas": 0
						}
					]
				}
			`,
		},
		{
			name: "replicas wrong argument count",
			givenCaddyfile: `
				supervisor {
				  php-fpm --fpm-config=fpm-8.0.2.ini {
					replicas
				  }
				}
			`,
			expectError: "wrong argument count or unexpected line ending after 'replicas', at Testfile:4",
		},
		{
			name: "replicas negative int",
			givenCaddyfile: `
				supervisor {
				  php-fpm --fpm-config=fpm-8.0.2.ini {
					replicas -1
				  }
				}
			`,
			expectError: "'replicas' should be a positive integer, '-1' given, at Testfile:4",
		},
		{
			name: "replicas not parsable int",
			givenCaddyfile: `
				supervisor {
				  php-fpm --fpm-config=fpm-8.0.2.ini {
					replicas hello
				  }
				}
			`,
			expectError: "'replicas' should be a positive integer, 'hello' given, at Testfile:4",
		},
		{
			name: "restart policy 'always'",
			givenCaddyfile: `
				supervisor {
				  php-fpm --fpm-config=fpm-8.0.2.ini {
					restart_policy always
				  }
				}
			`,
			expectJson: `
				{
					"supervise": [
						{
							"command":["php-fpm","--fpm-config=fpm-8.0.2.ini"],
							"restart_policy": "always"
						}
					]
				}
			`,
		},
		{
			name: "restart policy 'on_failure'",
			givenCaddyfile: `
				supervisor {
				  php-fpm --fpm-config=fpm-8.0.2.ini {
					restart_policy on_failure
				  }
				}
			`,
			expectJson: `
				{
					"supervise": [
						{
							"command":["php-fpm","--fpm-config=fpm-8.0.2.ini"],
							"restart_policy": "on_failure"
						}
					]
				}
			`,
		},
		{
			name: "stop signal parsing",
			givenCaddyfile: `
				supervisor {
				  php-fpm --fpm-config=fpm-8.0.2.ini {
				    stop_signal SIGQUIT
				  }
				}
			`,
			expectJson: `
				{
					"supervise": [
						{
							"command":["php-fpm","--fpm-config=fpm-8.0.2.ini"],
							"stop_signal": "SIGQUIT"
						}
					]
				}
			`,
		},
		{
			name: "restart policy wrong arguments count",
			givenCaddyfile: `
				supervisor {
				  php-fpm --fpm-config=fpm-8.0.2.ini {
					restart_policy
				  }
				}
			`,
			expectError: "wrong argument count or unexpected line ending after 'restart_policy', at Testfile:4",
		},
		{
			name: "restart policy invalid",
			givenCaddyfile: `
				supervisor {
				  php-fpm --fpm-config=fpm-8.0.2.ini {
					restart_policy foo
				  }
				}
			`,
			expectError: "'restart_policy' should be either 'always', 'never', or 'on_failure': 'foo' given, at Testfile:4",
		},
		{
			name: "termination_grace_period",
			givenCaddyfile: `
				supervisor {
				  php-fpm --fpm-config=fpm-8.0.2.ini {
					termination_grace_period 3m
				  }
				}
			`,
			expectJson: `
				{
					"supervise": [
						{
							"command":["php-fpm","--fpm-config=fpm-8.0.2.ini"],
							"termination_grace_period": "3m"
						}
					]
				}
			`,
		},
		{
			name: "termination_grace_period wrong argument count",
			givenCaddyfile: `
				supervisor {
				  php-fpm --fpm-config=fpm-8.0.2.ini {
					termination_grace_period
				  }
				}
			`,
			expectError: "wrong argument count or unexpected line ending after 'termination_grace_period', at Testfile:4",
		},
		{
			name: "termination_grace_period not parsable duration",
			givenCaddyfile: `
				supervisor {
				  php-fpm --fpm-config=fpm-8.0.2.ini {
					termination_grace_period foo
				  }
				}
			`,
			expectError: "cannot parse 'termination_grace_period' into time.Duration, 'foo' given, at Testfile:4",
		},
		{
			name: "dir",
			givenCaddyfile: `
				supervisor {
				  php-fpm --fpm-config=fpm-8.0.2.ini {
					dir /src
				  }
				}
			`,
			expectJson: `
				{
					"supervise": [
						{
							"command":["php-fpm","--fpm-config=fpm-8.0.2.ini"],
							"dir": "/src"
						}
					]
				}
			`,
		},
		{
			name: "dir wrong argument count",
			givenCaddyfile: `
				supervisor {
				  php-fpm --fpm-config=fpm-8.0.2.ini {
					dir
				  }
				}
			`,
			expectError: "wrong argument count or unexpected line ending after 'dir', at Testfile:4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := caddyfile.NewTestDispenser(tt.givenCaddyfile)

			res, err := parseSupervisor(d, nil)

			if tt.expectError != "" {
				assert.EqualError(t, err, tt.expectError)
			} else {
				assert.NoError(t, err)

				app, success := res.(httpcaddyfile.App)

				assert.True(t, success, "parseSupervisor result should be a httpcaddyfile.App")

				assert.Equal(t, "supervisor", app.Name)

				gotJson, err := app.Value.MarshalJSON()

				assert.NoError(t, err)
				assert.JSONEq(t, tt.expectJson, string(gotJson))
			}
		})
	}
}

// assertLoc fails the test if the condition is false.
func assertLoc(tb testing.TB, condition bool, msg string, v ...interface{}) {
	if !condition {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d: "+msg+"\033[39m\n\n", append([]interface{}{filepath.Base(file), line}, v...)...)
		tb.FailNow()
	}
}

// ok fails the test if an err is not nil.
func ok(tb testing.TB, err error) {
	if err != nil {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d: unexpected error: %s\033[39m\n\n", filepath.Base(file), line, err.Error())
		tb.FailNow()
	}
}

// equals fails the test if exp is not equal to act.
func equals(tb testing.TB, exp, act interface{}) {
	if !reflect.DeepEqual(exp, act) {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d:\n\n\texp: %#v\n\n\tgot: %#v\033[39m\n\n", filepath.Base(file), line, exp, act)
		tb.FailNow()
	}
}
