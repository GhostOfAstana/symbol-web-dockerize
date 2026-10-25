package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	assets "github.com/GhostOfAstana/symbol-web-dockerize"
	"github.com/GhostOfAstana/symbol-web-dockerize/internal/handlers"
	"github.com/GhostOfAstana/symbol-web-dockerize/internal/monitor"
	"github.com/GhostOfAstana/symbol-web-dockerize/internal/textai"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	probe := flag.Bool("healthcheck", false, "проверить доступность запущенного HTTP-сервера")
	flag.Parse()
	address := os.Getenv("ADDR")
	if address == "" {
		address = ":8080"
	}
	if *probe {
		return healthcheck(address)
	}
	mux, err := handlers.New(assets.FS, monitor.NewClient(), os.Getenv("MONITOR_CONTAINER"))
	if err != nil {
		return fmt.Errorf("подготовка сервера: %w", err)
	}
	assistant, err := textai.NewFromEnv()
	if err != nil {
		return fmt.Errorf("настройки помощника: %w", err)
	}
	assistant.Register(mux)
	server := &http.Server{Addr: address, Handler: handlers.SecureHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second,
		IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errorsCh := make(chan error, 1)
	go func() {
		log.Printf("Сервер доступен по адресу %s", address)
		errorsCh <- server.ListenAndServe()
	}()
	select {
	case err := <-errorsCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
			return err
		}
		return nil
	}
}

func healthcheck(address string) error {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("ADDR должен иметь вид :8080 или 127.0.0.1:8080")
	}
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("проверка вернула HTTP %d", response.StatusCode)
	}
	return nil
}
