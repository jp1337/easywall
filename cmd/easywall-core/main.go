package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jp1337/easywall/config"
	"github.com/jp1337/easywall/internal/core"
	"github.com/jp1337/easywall/internal/shared"
)

func main() {
	configPath := flag.String("config", "/etc/easywall/easywall.toml", "path to core config file")
	// So a build can be checked rather than assumed. The version is written in
	// by the linker, and when that silently did nothing there was no way to see
	// it from outside the binary.
	showVersion := flag.Bool("version", false, "print the version and exit")
	// The documentation described this flag for a long time before it existed,
	// and the command it published exited 2. It exists now: the commented
	// default is embedded, so a host with nothing but this binary can produce
	// one. It never overwrites.
	writeConfig := flag.String("write-config", "",
		"write a commented default configuration to this path and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("easywall-core", shared.CurrentVersion)
		return
	}

	if *writeConfig != "" {
		if err := shared.WriteDefaultConfig(*writeConfig, config.Core); err != nil {
			fmt.Fprintln(os.Stderr, "easywall-core:", err)
			os.Exit(1)
		}
		fmt.Println("wrote", *writeConfig)
		return
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg, err := core.LoadConfig(*configPath)
	if err != nil {
		slog.Error("failed to load config", "path", *configPath, "error", err)
		os.Exit(1)
	}

	if err := cfg.Validate(); err != nil {
		slog.Error("invalid config", "error", err)
		os.Exit(1)
	}

	daemon, err := core.NewDaemon(cfg)
	if err != nil {
		slog.Error("failed to initialize daemon", "error", err)
		os.Exit(1)
	}

	go func() {
		if err := daemon.Start(); err != nil {
			slog.Error("daemon error", "error", err)
			os.Exit(1)
		}
	}()

	slog.Info("easywall-core started", "socket", cfg.SocketPath, "version", shared.CurrentVersion)

	// SIGHUP reloads the configuration, which is what the documentation has
	// always said it does. Until it was handled here the default disposition
	// applied and the signal terminated the daemon.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	reload := make(chan os.Signal, 1)
	signal.Notify(reload, syscall.SIGHUP)

	for {
		select {
		case <-reload:
			if err := cfg.Reload(); err != nil {
				slog.Error("config reload failed; keeping the running configuration",
					"path", *configPath, "error", err)
				continue
			}
			slog.Info("configuration reloaded", "path", *configPath)
		case <-quit:
			slog.Info("shutting down easywall-core")
			daemon.Stop()
			return
		}
	}
}
