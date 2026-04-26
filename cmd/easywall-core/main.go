package main

import (
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jp1337/easywall/internal/core"
)

func main() {
	configPath := flag.String("config", "/etc/easywall/easywall.toml", "path to core config file")
	flag.Parse()

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

	slog.Info("easywall-core started", "socket", cfg.SocketPath, "version", "2.0.0-dev")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit

	slog.Info("shutting down easywall-core")
	daemon.Stop()
}
