package supervisor

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const (
	helperEnv = "SUPERVISOR_TEST_HELPER_MARKER"
	waitFor   = 30 * time.Second
	// the helper writes <marker><stoppedSuffix> when it was asked to stop instead of being killed
	stoppedSuffix = ".stopped"
)

// TestMain lets the test binary act as the supervised process: started with helperEnv set, it records its pid in
// the marker file and then blocks until it is killed.
func TestMain(m *testing.M) {
	if marker := os.Getenv(helperEnv); marker != "" {
		f, err := os.OpenFile(marker, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o666)
		if err != nil {
			os.Exit(2)
		}
		fmt.Fprintf(f, "%d\n", os.Getpid())
		f.Close()

		// asked to stop (SIGINT, on Windows Ctrl+Break): leave a trace and exit cleanly
		interrupted := make(chan os.Signal, 1)
		signal.Notify(interrupted, os.Interrupt)
		select {
		case <-interrupted:
			_ = os.WriteFile(marker+stoppedSuffix, []byte("asked to stop"), 0o666)
		case <-time.After(time.Hour):
		}
		os.Exit(0)
	}

	os.Exit(m.Run())
}

// helperDefinition supervises the test binary itself; every start appends the pid to the returned marker file.
func helperDefinition(t *testing.T, onReload OnReload) (Definition, string) {
	t.Helper()
	marker := filepath.Join(t.TempDir(), "started")

	return Definition{
		Command:        []string{os.Args[0]},
		Env:            map[string]string{helperEnv: marker},
		RedirectStdout: &OutputTarget{Type: OutputTypeNull},
		RedirectStderr: &OutputTarget{Type: OutputTypeNull},
		OnReload:       onReload,
	}, marker
}

func helperSupervisor(t *testing.T) (*Supervisor, string) {
	t.Helper()
	def, marker := helperDefinition(t, "")
	supervisors, err := def.ToSupervisors(zap.NewNop())
	require.NoError(t, err)
	require.Len(t, supervisors, 1)

	return supervisors[0], marker
}

func newTestApp(t *testing.T, def Definition) *App {
	t.Helper()
	app := &App{Supervise: []Definition{def}, log: zap.NewNop()}
	supervisors, err := def.ToSupervisors(app.log)
	require.NoError(t, err)
	app.supervisors = supervisors

	return app
}

// startedPids returns the pids the helper wrote, one per start.
func startedPids(marker string) []string {
	content, err := os.ReadFile(marker)
	if err != nil {
		return nil
	}

	return strings.Fields(string(content))
}

func waitForStarts(t *testing.T, marker string, count int) []string {
	t.Helper()
	require.Eventually(t, func() bool { return len(startedPids(marker)) >= count }, waitFor, 20*time.Millisecond,
		"expected %d process start(s)", count)

	return startedPids(marker)
}

func runInBackground(s *Supervisor) chan struct{} {
	returned := make(chan struct{})
	go func() {
		s.Run()
		close(returned)
	}()

	return returned
}

// A config reload stops the previous app right after starting it. Stop() used to be a no-op when the process was
// not running yet, and Run() then reset the flag and started a process nobody would ever stop again.
func TestSupervisor_StopBeforeRunStartsNothing(t *testing.T) {
	s, marker := helperSupervisor(t)

	s.Stop()
	returned := runInBackground(s)

	select {
	case <-returned:
	case <-time.After(waitFor):
		s.Stop()
		t.Fatal("Run() must return without starting the process once the supervisor was stopped")
	}
	assert.Empty(t, startedPids(marker), "a stopped supervisor must not start its process")
}

func TestSupervisor_StopWaitsUntilTheProcessIsGone(t *testing.T) {
	s, marker := helperSupervisor(t)
	returned := runInBackground(s)
	waitForStarts(t, marker, 1)

	s.Stop()

	// Stop() returned: the process has exited and the supervising loop is done - no overlap with a successor.
	select {
	case <-returned:
	default:
		t.Fatal("Stop() must not return before the process has exited")
	}
	assert.Len(t, startedPids(marker), 1, "a stopped process must not be restarted")
}

// The process gets the chance to shut down by itself before it is killed - on Windows too, where there are no
// signals and the request is a Ctrl+Break sent to the process group of the command.
func TestSupervisor_StopAsksTheProcessToStopBeforeKillingIt(t *testing.T) {
	s, marker := helperSupervisor(t)
	returned := runInBackground(s)
	waitForStarts(t, marker, 1)

	s.Stop()

	<-returned
	assert.FileExists(t, marker+stoppedSuffix, "the process must be asked to stop, not killed right away")
}

func TestSupervisor_StopInterruptsTheRestartDelay(t *testing.T) {
	def := Definition{
		Command:        []string{filepath.Join(t.TempDir(), "does-not-exist")},
		RedirectStdout: &OutputTarget{Type: OutputTypeNull},
		RedirectStderr: &OutputTarget{Type: OutputTypeNull},
	}
	supervisors, err := def.ToSupervisors(zap.NewNop())
	require.NoError(t, err)
	returned := runInBackground(supervisors[0])

	// let it fail a few times so that the delay before the next attempt is several seconds long
	time.Sleep(3500 * time.Millisecond)
	stopStarted := time.Now()
	supervisors[0].Stop()

	select {
	case <-returned:
	case <-time.After(waitFor):
		t.Fatal("Run() must return when stopped while waiting for the next start attempt")
	}
	assert.Less(t, time.Since(stopStarted), time.Second, "Stop() must not sit out the restart delay")
}

func TestApp_OnReloadRestartStartsANewProcessPerConfig(t *testing.T) {
	def, marker := helperDefinition(t, "")
	previous := newTestApp(t, def)
	require.NoError(t, previous.Start())
	waitForStarts(t, marker, 1)

	// reload: the new config starts first, then the previous one is stopped
	next := newTestApp(t, def)
	require.NoError(t, next.Start())
	pids := waitForStarts(t, marker, 2)
	require.NoError(t, previous.Stop())

	assert.NotEqual(t, pids[0], pids[1], "default behaviour: every config generation runs its own process")
	require.NoError(t, next.Stop())
	assert.Len(t, startedPids(marker), 2)
}

func TestApp_OnReloadKeepLeavesAnUnchangedProcessRunning(t *testing.T) {
	def, marker := helperDefinition(t, OnReloadKeep)
	previous := newTestApp(t, def)
	require.NoError(t, previous.Start())
	waitForStarts(t, marker, 1)

	// two reloads in quick succession, as an admin API client changing two config values would cause
	second := newTestApp(t, def)
	require.NoError(t, second.Start())
	third := newTestApp(t, def)
	require.NoError(t, third.Start())
	require.NoError(t, previous.Stop())
	require.NoError(t, second.Stop())

	time.Sleep(300 * time.Millisecond)
	assert.Len(t, startedPids(marker), 1, "the process must survive reloads untouched")
	assert.True(t, third.supervisors[0].isRunning(), "the current config supervises the process started earlier")

	require.NoError(t, third.Stop())
	assert.False(t, third.supervisors[0].isRunning(), "the process stops once no config uses it anymore")
	assert.Len(t, startedPids(marker), 1)
}

func TestApp_OnReloadKeepRestartsAChangedDefinition(t *testing.T) {
	def, marker := helperDefinition(t, OnReloadKeep)
	previous := newTestApp(t, def)
	require.NoError(t, previous.Start())
	waitForStarts(t, marker, 1)

	changed := def
	changed.Env = map[string]string{helperEnv: marker, "CHANGED": "1"}
	next := newTestApp(t, changed)
	require.NoError(t, next.Start())
	pids := waitForStarts(t, marker, 2)
	require.NoError(t, previous.Stop())

	assert.NotEqual(t, pids[0], pids[1], "a changed definition is a different process")
	assert.False(t, previous.supervisors[0].isRunning(), "the process of the previous definition is stopped")
	assert.True(t, next.supervisors[0].isRunning())
	require.NoError(t, next.Stop())
}

func TestOptions_KeyIsStableAndTellsDefinitionsApart(t *testing.T) {
	key := func(def Definition) string {
		supervisors, err := def.ToSupervisors(zap.NewNop())
		require.NoError(t, err)

		return supervisors[0].Options.key()
	}
	base := Definition{Command: []string{"php", "artisan", "queue:work"}, Env: map[string]string{"A": "1", "B": "2", "C": "3"}}

	for i := 0; i < 20; i++ {
		assert.Equal(t, key(base), key(base), "the key must not depend on map iteration order")
	}

	otherArgs := base
	otherArgs.Command = []string{"php", "artisan", "queue:listen"}
	otherDir := base
	otherDir.Dir = "/srv/app"
	assert.NotEqual(t, key(base), key(otherArgs))
	assert.NotEqual(t, key(base), key(otherDir))

	two := 2
	replicated := base
	replicated.Replicas = &two
	supervisors, err := replicated.ToSupervisors(zap.NewNop())
	require.NoError(t, err)
	assert.NotEqual(t, supervisors[0].Options.key(), supervisors[1].Options.key(), "replicas are separate processes")
}
