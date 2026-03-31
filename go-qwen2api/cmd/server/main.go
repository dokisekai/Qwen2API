package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Rfym21/Qwen2API/go-qwen2api/internal/account"
	"github.com/Rfym21/Qwen2API/go-qwen2api/internal/config"
	"github.com/Rfym21/Qwen2API/go-qwen2api/internal/logger"
	"github.com/Rfym21/Qwen2API/go-qwen2api/internal/proxy"
	"github.com/Rfym21/Qwen2API/go-qwen2api/internal/route"
)

func main() {
	config.Load()

	if len(config.C.APIKeys) == 0 {
		fmt.Fprintln(os.Stderr, "ERROR: API_KEY environment variable is required")
		os.Exit(1)
	}

	logger.Init(config.C.LogLevel, config.C.EnableFileLog, config.C.LogDir, config.C.MaxLogFileSize, config.C.MaxLogFiles)
	logger.Info("SERVER", "Qwen2API Go Server (CLI mode) starting...")
	logger.Info("SERVER", "admin key: %s", config.C.AdminKey)

	proxy.Init()
	account.Init()

	router := route.Setup()

	addr := config.C.ListenAddress
	port := config.C.ListenPort
	if addr == "" {
		addr = "0.0.0.0"
	}
	listen := fmt.Sprintf("%s:%d", addr, port)

	srv := &http.Server{
		Addr:         listen,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		logger.Info("SERVER", "listening on %s", listen)
		logger.Info("SERVER", "dataSaveMode=%s maxCliRequests=%d", config.C.DataSaveMode, config.C.MaxCliRequests)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("SERVER", "server error: %v", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	logger.Info("SERVER", "received signal: %v, shutting down...", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("SERVER", "shutdown error: %v", err)
	}
	logger.Info("SERVER", "server stopped")
}
