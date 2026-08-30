// Copyright IBM Corp. 2021, 2023
// SPDX-License-Identifier: MPL-2.0

package view

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hashicorp/nomad/api"
	"golang.org/x/term"
)

// Exec suspends the TUI and opens an interactive shell inside the given
// task/allocation, equivalent to `nomad alloc exec -task <task> <allocID> /bin/sh`.
func (v *View) Exec(taskName, allocID string) {
	var execErr error

	v.Layout.Container.Suspend(func() {
		// tcell may send terminal queries (e.g. cursor position reports) as
		// part of tearing down the screen; the terminal's reply can arrive
		// just after control is handed over here, and would otherwise be
		// read by the remote shell as literal input. Drain it first.
		drainStdin(100 * time.Millisecond)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		resizeCh := make(chan api.TerminalSize, 1)
		if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
			resizeCh <- api.TerminalSize{Width: w, Height: h}
		}

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGWINCH)
		defer signal.Stop(sigCh)

		go func() {
			for range sigCh {
				if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
					select {
					case resizeCh <- api.TerminalSize{Width: w, Height: h}:
					default:
					}
				}
			}
		}()

		_, execErr = v.Client.Exec(ctx, allocID, taskName, []string{"/bin/sh"},
			os.Stdin, os.Stdout, os.Stderr, resizeCh)
	})

	if execErr != nil {
		v.handleError("exec failed: %s", execErr.Error())
	}

	v.Draw()
}

// drainStdin discards any bytes already buffered on stdin, for up to
// timeout, without blocking indefinitely if nothing arrives.
func drainStdin(timeout time.Duration) {
	if err := os.Stdin.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		// Deadlines aren't supported on this stdin (e.g. not a pollable
		// file); draining would risk blocking forever, so skip it.
		return
	}
	defer os.Stdin.SetReadDeadline(time.Time{})

	buf := make([]byte, 256)
	for {
		n, err := os.Stdin.Read(buf)
		if n <= 0 || err != nil {
			return
		}
	}
}
