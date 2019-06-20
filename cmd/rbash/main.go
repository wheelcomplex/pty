// run first aviable shell with pty

package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/wheelcomplex/filetype"
	"github.com/wheelcomplex/permbits"
	"github.com/wheelcomplex/pty"
	"golang.org/x/crypto/ssh/terminal"
)

func getFileType(path string) (ct string, err error) {

	ct = "application/octet-stream"
	permissions, err := permbits.Stat(path)
	if err != nil {
		// log.Printf("file %s stat error: %s\n", path, err)
		return ct, err
	}
	file, err := os.Open(path)
	if err != nil {
		return ct, err
	}
	defer file.Close()

	// Only the first 512 bytes are used to sniff the content type.
	buffer := make([]byte, 512)
	_, err = file.Read(buffer)
	if err != nil {
		return ct, err
	}

	kind, _ := filetype.Match(buffer)
	log.Printf("%s is identify as %s, %s by filetype.Match\n", path, kind.Extension, kind.MIME.Value)

	if kind.Extension == "elf" && kind.MIME.Value == "application/x-executable" && permissions.UserExecute() {
		return "application/x-elf-executable", nil
	}
	return ct, nil
}

func runshell() error {
	// Create arbitrary command.

	log.SetOutput(os.Stderr)
	log.SetPrefix(" - initz, ")

	shells := [...]string{
		"/bin/bash",
		"/root/bin/bash",
		"/bin/sh",
		"/root/bin/sh",
	}
	shell := ""
	var shellErr error = nil
	for _, shell = range shells {
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
		shellErr = err
		if err == nil {
			break
		}
	}

	if shellErr != nil {
		log.Printf("shells (%d) not found, exit ...\n", len(shells))
		return shellErr
	}

	cmdline := ""
	for _, arg := range os.Args[1:] {
		cmdline += arg + " "
	}
	cmdline = strings.Trim(cmdline, " ")

	var firstArg string = ""
	if len(cmdline) > 0 {
		log.Printf("SHELL: %s %s\n", shell, cmdline)
		firstArg = strings.Trim(os.Args[1], " ")
	} else {
		log.Printf("SHELL: %s\n", shell)
	}

	// lookup
	path, err := exec.LookPath(firstArg)
	if err != nil {
		// log.Printf("%s not found in PATH\n", firstArg)
		firstArg = ""
	} else {
		// log.Printf("%s found in PATH: %s\n", firstArg, path)

		// check for script file
		fileType, err := getFileType(path)

		if err == nil && fileType == "application/x-elf-executable" {
			firstArg = path
		} else {
			firstArg = ""
		}
	}

	var c *exec.Cmd

	if len(firstArg) > 0 {
		// binary file, execute directly
		if len(os.Args) >= 3 {
			c = exec.Command(firstArg, os.Args[2:]...)
		} else {
			c = exec.Command(firstArg)
		}
	} else {
		// script file, execute by shell
		c = exec.Command(shell, os.Args[1:]...)
	}

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
	if err := runshell(); err != nil {
		log.Fatal(err)
	}
}
