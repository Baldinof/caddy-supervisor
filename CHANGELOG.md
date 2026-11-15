# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Graceful stop on Windows: the command runs in its own process group and is asked to stop with `Ctrl+Break`; `termination_grace_period` applies. It was killed immediately before.
- `on_reload` option: `keep` leaves a command with an unchanged definition running across config reloads instead of restarting it.

### Fixed
- A command was left running forever, unsupervised by the active config, if the config was reloaded again before the command had been started (e.g. two changes through the admin API in a row). `Stop()` did nothing while no process was running, and the supervising loop then started the process anyway.
- `Stop()` returns only after the process has exited. Previously, where processes cannot be signaled (Windows), it killed the process and returned immediately.
- Stopping no longer waits for a pending restart delay (up to 5 minutes).
- The replica number in log messages was always 0.

## [1.0.0] - 2021-07-12

### Added
- Execute command as a different user. Thank you [@mmachatschek](https://github.com/mmachatschek), see [#1](https://github.com/Baldinof/caddy-supervisor/pull/1)

### Fixed
- Panic if only redirect_stdout is provided in caddyfile. Thank you [@mmachatschek](https://github.com/mmachatschek), see [#2](https://github.com/Baldinof/caddy-supervisor/pull/2)
