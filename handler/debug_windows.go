package handler

import (
	"context"
	"io"
)

func runDebugCommands(w io.Writer) error {
	return nil
}

func runPing(ctx context.Context, deviceName, remoteIP string, w io.Writer, count int) error {
	return nil
}

func runPacketCapture(ctx context.Context, deviceName, filename string) error {
	return nil
}
