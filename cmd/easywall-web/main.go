package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jp1337/easywall/config"
	"github.com/jp1337/easywall/internal/shared"
	"github.com/jp1337/easywall/internal/web"
)

func main() {
	configPath := flag.String("config", "/etc/easywall/web.toml", "path to web config file")
	// So a build can be checked rather than assumed — see easywall-core.
	showVersion := flag.Bool("version", false, "print the version and exit")
	// See easywall-core: the commented default is embedded and this never
	// overwrites, which matters more here — web.toml holds the session key and
	// the password hash.
	writeConfig := flag.String("write-config", "",
		"write a commented default configuration to this path and exit")
	// The container's HEALTHCHECK. It reads bind_addr the way the server does —
	// file, then EASYWALL_WEB_BIND_ADDR — and writes nothing.
	healthcheck := flag.Bool("healthcheck", false,
		"ask /healthz at bind_addr and exit 0 on a 200, 1 otherwise")
	flag.Parse()

	if *showVersion {
		fmt.Println("easywall-web", shared.CurrentVersion)
		return
	}

	if *writeConfig != "" {
		if err := shared.WriteDefaultConfig(*writeConfig, config.Web); err != nil {
			fmt.Fprintln(os.Stderr, "easywall-web:", err)
			os.Exit(1)
		}
		fmt.Println("wrote", *writeConfig)
		return
	}

	if *healthcheck {
		cfg, err := web.LoadConfig(*configPath)
		if err == nil {
			err = web.HealthCheck(cfg)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "easywall-web:", err)
			os.Exit(1)
		}
		return
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg, err := web.LoadConfig(*configPath)
	if err != nil {
		slog.Error("failed to load config", "path", *configPath, "error", err)
		os.Exit(1)
	}

	if err := cfg.Validate(); err != nil {
		slog.Error("invalid config", "error", err)
		os.Exit(1)
	}

	srv, err := web.NewServer(cfg)
	if err != nil {
		slog.Error("failed to initialize server", "error", err)
		os.Exit(1)
	}

	go func() {
		if err := srv.Start(); err != nil {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	slog.Info("easywall-web started", "addr", cfg.BindAddr, "version", shared.CurrentVersion)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit

	slog.Info("shutting down easywall-web")
	srv.Stop()
}
