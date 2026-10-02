package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"bitbucket.org/lomoware/lomo-backend/handler/lomocloud"
	"github.com/sirupsen/logrus"
)

func signalHandler(h *lomocloud.Handler, cancel context.CancelFunc) {
	sigChan := make(chan os.Signal, 4)

	signal.Notify(
		sigChan,
		syscall.SIGUSR1,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGHUP,
	)
	for {
		sig, ok := <-sigChan
		if !ok {
			return
		}

		switch sig {
		case syscall.SIGTERM, syscall.SIGINT, syscall.SIGUSR1, syscall.SIGHUP:
			cancel()
			if err := h.Close(); err != nil {
				logrus.Fatal(err)
			}
			logrus.Info("Handler is closed")
			os.Exit(0)
		}
	}
}
