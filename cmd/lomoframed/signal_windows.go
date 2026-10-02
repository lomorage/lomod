package main

import (
	"context"
	"os"
	"os/signal"

	"bitbucket.org/lomoware/lomo-backend/handler/lomoframe"
	"github.com/sirupsen/logrus"
)

// Refer https://golang.org/src/os/signal/signal_windows_test.go
func signalHandler(h *lomoframe.Handler, cancel context.CancelFunc) {
	sigChan := make(chan os.Signal, 10)

	signal.Notify(sigChan)

	for {
		sig, ok := <-sigChan
		if !ok {
			return
		}

		switch sig {
		case os.Interrupt:
			cancel()

			if err := h.Close(); err != nil {
				logrus.Fatal(err)
			}
			logrus.Info("Handler is closed")
			os.Exit(0)
		default:
			logrus.Errorf("Wrong signal received: got %q, expect %q\n", sig, os.Interrupt)
			os.Exit(1)
		}
	}
}
