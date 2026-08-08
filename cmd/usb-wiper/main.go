package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/usb-wiper/internal/server"
)

var version = "dev"

func main() {
	port := envOrDefault("PORT", "8181")
	dataDir := envOrDefault("DATA_DIR", "/data")
	unsafeAllowAllUSB := os.Getenv("UNSAFE_ALLOW_ALL_USB") == "1"

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("USB Wiper %s", version)
	log.Printf("port=%s data_dir=%s unsafe_allow_all_usb=%v", port, dataDir, unsafeAllowAllUSB)

	if unsafeAllowAllUSB {
		log.Println("⚠ WARNING: UNSAFE_ALLOW_ALL_USB=1 — removable check is DISABLED. Be careful!")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	srv, err := server.New(port, unsafeAllowAllUSB, dataDir)
	if err != nil {
		log.Printf("startup failed: %v", err)
		os.Exit(1)
	}
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
