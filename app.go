package supervisor

import (
	"github.com/caddyserver/caddy/v2"
	"go.uber.org/zap"
)

// Interface guards
var (
	_ caddy.App         = (*App)(nil)
	_ caddy.Module      = (*App)(nil)
	_ caddy.Provisioner = (*App)(nil)
	_ caddy.Destructor  = (*Supervisor)(nil)
)

// keptSupervisors holds the supervisors of definitions with "on_reload keep". They live across config reloads:
// Caddy starts the apps of the new config before it stops the apps of the previous one, so the new app takes over
// the running supervisor here and the previous app merely drops its reference.
var keptSupervisors = caddy.NewUsagePool()

func init() {
	caddy.RegisterModule(App{})
}

type App struct {
	Supervise   []Definition `json:"supervise,omitempty"`
	log         *zap.Logger
	supervisors []*Supervisor
}

// CaddyModule implements caddy.Module
func (a App) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "supervisor",
		New: func() caddy.Module { return new(App) },
	}
}

// Provision implements caddy.Provisioner
func (a *App) Provision(context caddy.Context) error {
	a.log = context.Logger(a)

	for _, definition := range a.Supervise {

		supervisors, err := definition.ToSupervisors(a.log)

		if err != nil {
			return err
		}

		a.supervisors = append(a.supervisors, supervisors...)
	}

	a.log.Debug("module provisioned", zap.Any("supervisors", a.supervisors))

	return nil
}

// Start implements caddy.App
func (a *App) Start() error {
	for i, s := range a.supervisors {
		if s.Options.OnReload != OnReloadKeep {
			go s.Run()

			continue
		}

		created := s
		value, loaded, err := keptSupervisors.LoadOrNew(s.Options.key(), func() (caddy.Destructor, error) {
			return created, nil
		})
		if err != nil {
			return err
		}

		if !loaded {
			go s.Run()

			continue
		}

		// same definition as in the previous config: its process keeps running, supervised from now on by this app
		kept := value.(*Supervisor)
		kept.setLogger(s.log())
		a.supervisors[i] = kept

		kept.log().Info("process kept running across config reload")
	}

	a.log.Debug("module started")

	return nil
}

// Stop implements caddy.App
func (a *App) Stop() error {
	for _, s := range a.supervisors {
		if s.Options.OnReload != OnReloadKeep {
			s.Stop()

			continue
		}

		// stops the process only if no other (newer) config uses it
		if _, err := keptSupervisors.Delete(s.Options.key()); err != nil {
			return err
		}
	}

	return nil
}
