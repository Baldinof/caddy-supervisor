# Caddy Supervisor 

[![Build Status](https://github.com/baldinof/caddy-supervisor/actions/workflows/ci.yaml/badge.svg)](hthttps://github.com/Baldinof/caddy-supervisor/actions/workflows/ci.yaml)

A module to run and supervise background processes from Caddy

## How it works

For every process in the **supervisor** caddyfile directive a command is executed in background and killed when caddy stops.

## Full HTTP Cadyfile example

```Caddyfile
{
  # Must be in global options
  supervisor {
    php-fpm --nodaemonize {
      dir /path/to/desired/working-dir # default to current dir
      
      env APP_ENV production
      env DEBUG false
      
      user www-data
      
      restart_policy always # default to 'always', other values allowed: 'never', 'on_failure'
      
      redirect_stdout file /var/log/fpm.log       # redirect command stdout to a file. Default to caddy `stdout`
      redirect_stderr file /var/log/fpm-error.log # redirect command stderr to a file. Default to caddy `stderr`
      
      termination_grace_period 30s # default to '10s', amount of time to wait for application graceful termination before killing it
      
      replicas 3 # default to 1, number of instances that should be executed
    }
    
    # block configuration is optional    
    node worker.js
  }
}

mysite.com
```

## Options description

- **command**: the command to be executed. _Supports template_.
- **dir**: the working directory the command should be executed in. _Supports template_.
- **env**: declare environment variable that should be passed to command. This property can be repeated. _Supports template_.
- **redirect_stdout**: redirect command stdout. Default: `stdout`, Possible values:
  - **null**: discard output
  - **stdout**: redirect to the caddy process stdout
  - **stderr**: redirect to the caddy process stderr
  - **file /path/to/file**: redirect output to a file
- **redirect_stderr**: redirect command stderr. Default: `stderr`, See above for possible values.
- **restart_policy**: define under which conditions the command should be restarted after exit. Default: `always` Valid values:
  - **never**: do not restart the command
  - **on_failure**: restart if exit code is not 0
  - **always**: always restart
- **termination_grace_period**: amount of time to wait for application graceful termination before killing it. Ex: 10s
- **replicas**: number of instances that should be executed. Default: 1.
- **user**: the user that runs the command. _Ignored on windows systems_

On windows **termination_grace_period** is ignored and the command is killed immediatelly due to lack of signals support.

## Templates
To enable different configuration per replica, you can use go templates on the fields marked with _Supports template_".

The following information are available to templates:
- **Replica**: the index of the current replica, starting from 0

Templates also supports all functions from http://masterminds.github.io/sprig/

Example:
```
{
  supervisor {
    myapp --port "{{add 8000 .Replica}}" {
      replicas 5
    }
  }
}

reverse_proxy * localhost:8000-8004
```

## Exponential backoff
To avoid spending too many resources on a crashing application, this plugin makes use of exponential backoff.

That means that when the command fail, it will be restarted with a delay of 0 seconds. If it fails again it will be restarted with a delay of 1 seconds, then on every sucessive failure the delay time doubles, with a max limit of 5 minutes.

If the command runs stable for at least 10 minutes, the restart delay is reset to 0 seconds.

## Examples

PHP server (useful if you want a single container PHP application with php-fpm):

```Caddyfile
{
  supervisor {
    php-fpm
  }
}

example.com

php_fastcgi 127.0.0.1:9000
root * .
encode gzip
file_server
```

AspNet Core application on windows:

```
{
  supervisor {
    dotnet ./MyApplication.dll {
      env ASPNETCORE_URLS http://localhost:5000
      dir "C:\MyApplicationFolder"
      redirect_stdout stdout
      redirect_stderr stderr
      restart_policy always
    }
  }
}

example.com

reverse_proxy localhost:5000
```

## Building it

Use the `xcaddy` tool to build a version of caddy with this module:

```
xcaddy build \
    --with github.com/baldinof/caddy-supervisor
```

## Todo

- Dedicated Caddyfile support (Supervisorfile)
- Send processes outputs to Caddy logs

## Credits

This package is continuation of https://github.com/lucaslorentz/caddy-supervisor which only supports Caddy v1.

Thank you @lucaslorentz for the original work ❤️
