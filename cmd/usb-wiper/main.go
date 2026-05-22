package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/usb-wiper/internal/server"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	port := envOrDefault("PORT", "8181")
	logLevel := envOrDefault("LOG_LEVEL", "info")
	dataDir := envOrDefault("DATA_DIR", "/data")
	unsafeAllowAllUSB := os.Getenv("UNSAFE_ALLOW_ALL_USB") == "1"

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("USB Wiper %s (built %s)", version, buildTime)
	log.Printf("port=%s log_level=%s data_dir=%s unsafe_allow_all_usb=%v", port, logLevel, dataDir, unsafeAllowAllUSB)

	if unsafeAllowAllUSB {
		log.Println("⚠ WARNING: UNSAFE_ALLOW_ALL_USB=1 — removable check is DISABLED. Be careful!")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	srv := server.New(port, unsafeAllowAllUSB, dataDir)
	if err := srv.Start(ctx); err != nil {
		log.Printf("shutting down: %v", err)
		os.Exit(1)
	}
}

func envOrDefault(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}
