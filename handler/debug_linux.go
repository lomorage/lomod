package handler

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"bitbucket.org/lomoware/lomo-backend/common/cmd"
	"github.com/pkg/errors"
)

var commands = [][]string{{"mount"}, {"df", "-h"}, {"sudo", "fdisk", "-l"}}

func runDebugCommands(w io.Writer) error {
	for _, command := range commands {
		if _, err := w.Write([]byte(fmt.Sprintf("$ %v\n", command))); err != nil {
			return err
		}
		output, err := cmd.Run(command[0], command[1:]...)
		if err != nil {
			if _, err2 := w.Write([]byte(err.Error())); err2 != nil {
				return errors.Wrapf(err2, err.Error())
			}
		}
		if _, err := w.Write(output); err != nil {
			return err
		}
	}
	return nil
}

func runPing(ctx context.Context, deviceName, remoteIP string, w io.Writer, count int) error {
	return cmd.RunContextWriter(ctx, w, "ping", "-c", strconv.Itoa(count), "-I", deviceName, remoteIP)
}

func runPacketCapture(ctx context.Context, deviceName, filename string) error {
	out, err := cmd.RunContext(ctx, "sudo", "tcpdump", "-w", filename, "-i", deviceName, "udp", "port", "5353",
		"or", "tcp", "port", "8000")
	if err != nil {
		return errors.Wrapf(err, string(out))
	}
	return nil
}
