package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/diagnostics"
	"github.com/OhRats-Technologies/rc/pkg/kernel"
	"github.com/OhRats-Technologies/rc/pkg/node"
	"github.com/OhRats-Technologies/rc/pkg/platform"
)

func runRun(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	stateDir := fs.String("state-dir", "", "Override state directory")
	componentDir := fs.String("component-dir", "", "Override component directory")
	once := fs.Bool("once", false, "Send one heartbeat and exit")
	interval := fs.Duration("interval", 30*time.Second, "Heartbeat interval")
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return err
	}
	if *interval < time.Second {
		return fmt.Errorf("heartbeat interval must be at least 1s")
	}
	dir, err := node.ResolveStateDir(*stateDir)
	if err != nil {
		return err
	}
	state, err := node.LoadState(dir)
	if err != nil {
		return node.ErrNotEnrolled
	}
	config, _ := node.LoadConfig(dir)
	server := config.Server
	if server == "" {
		server = os.Getenv("RC_URL")
	}
	if server == "" {
		server = "https://rc.ohrats.party"
	}

	lock, err := platform.AcquireRunLock(dir)
	if err != nil {
		return err
	}
	defer lock.Close()

	journal, _ := platform.NewJournal(dir)
	logEvent := func(message string) {
		if journal != nil {
			_ = journal.Append(message)
		}
		_ = diagnostics.Append(dir, diagnostics.Event{Level: "info", Source: "node", Code: "node.lifecycle", Message: message})
	}
	logEvent("node started")
	defer logEvent("node stopped")

	execMgr := node.NewNativeExecutionManager()
	defer execMgr.Shutdown()
	controlMgr := node.NewControlManager(state, dir, execMgr)

	components, err := resolveDirectory("components", *componentDir)
	if err != nil {
		return err
	}
	runtimeHandle, err := kernel.NewRuntime(components)
	if err != nil {
		return err
	}
	defer runtimeHandle.Close()

	send := func() {
		heartbeatCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		if err := node.Heartbeat(heartbeatCtx, server, state); err != nil {
			logEvent("heartbeat failed: " + err.Error())
			return
		}
		logEvent("heartbeat ok")
	}
	send()
	if *once {
		return nil
	}
	controlCtx, stopControl := context.WithCancel(ctx)
	defer stopControl()
	go node.RunControl(controlCtx, server, dir, state, execMgr, logEvent, controlMgr)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)
	heartbeat := time.NewTicker(*interval)
	defer heartbeat.Stop()
	reconcile := time.NewTicker(2 * time.Second)
	defer reconcile.Stop()
	for {
		select {
		case <-stop:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-heartbeat.C:
			send()
		case <-reconcile.C:
			changed, err := runtimeHandle.Reconcile()
			if err != nil {
				logEvent("component reconcile failed: " + err.Error())
				continue
			}
			if changed {
				logEvent(fmt.Sprintf("components reconciled: %s", runtimeHandle.Summary()))
			}
			for path, message := range runtimeHandle.FailedPaths() {
				logEvent(fmt.Sprintf("component %s failed: %s", filepath.Base(path), message))
			}
		}
	}
}
