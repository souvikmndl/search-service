package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/souvikmndl/search-service/internal/config"
	"github.com/souvikmndl/search-service/internal/handler"
)

func main() {
	slog.Info("starting app by loading config")
	config, err := config.New()
	if err != nil {
		log.Fatalf("error loading config %v", err)
	}
	slog.Info("Config", "value", config)
	e, sqlDB := handler.InitService(config)
	defer sqlDB.Close()

	go func() {
		log.Println("Starting server on port :3000")
		if err := e.Start(fmt.Sprintf(":%d", config.Server.Port)); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Graceful Shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := e.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server exited properly")
}
