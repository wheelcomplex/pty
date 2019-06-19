package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/wheelcomplex/permbits"
	"github.com/wheelcomplex/pty"
	"golang.org/x/crypto/ssh/terminal"
)

func test() error {
	// Create arbitrary command.

	log.SetOutput(os.Stderr)
	log.SetPrefix(" - initz, ")

	shell := "/bin/bash"

	permissions, err := permbits.Stat(shell)
	if err != nil {
		log.Printf("file %s stat error: %s, try next ...\n", shell, err)
	} else {
		if permissions.UserExecute() == false {
			err = fmt.Errorf("%s is not executable\n", shell)
		} else if permissions.UserRead() == false {
			err = fmt.Errorf("%s is not readable\n", shell)
		}
	}

	if err != nil {
		shell = "/root/bin/bash"
		permissions, err = permbits.Stat(shell)
		if err != nil {
			log.Printf("file %s stat error: %s, exit ...\n", shell, err)
			return err
		}
		if permissions.UserExecute() == false {
			err = fmt.Errorf("%s is not executable\n", shell)
			return err
		} else if permissions.UserRead() == false {
			err = fmt.Errorf("%s is not readable\n", shell)
			return err
		}
	}
	log.Printf("SHELL: %s\n", shell)

	c := exec.Command(shell)

	// Start the command with a pty.
	ptmx, err := pty.Start(c)
	if err != nil {
		return err
	}
	// Make sure to close the pty at the end.
	defer func() { _ = ptmx.Close() }() // Best effort.

	// Handle pty size.
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
				log.Printf("error resizing pty: %s", err)
			}
		}
	}()
	ch <- syscall.SIGWINCH // Initial resize.

	// Set stdin in raw mode.
	oldState, err := terminal.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}
	defer func() { _ = terminal.Restore(int(os.Stdin.Fd()), oldState) }() // Best effort.

	// Copy stdin to the pty and the pty to stdout.
	go func() { _, _ = io.Copy(ptmx, os.Stdin) }()
	_, _ = io.Copy(os.Stdout, ptmx)

	return nil
}

func main() {
	if err := test(); err != nil {
		log.Fatal(err)
	}
}
