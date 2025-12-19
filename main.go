package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
)

type LinePrefixWriter struct {
	dst         io.Writer
	prefix      []byte
	atLineStart bool
}

func NewLinePrefixWriter(dst io.Writer, prefix string) *LinePrefixWriter {
	return &LinePrefixWriter{
		dst:         dst,
		prefix:      []byte(prefix),
		atLineStart: true,
	}
}

func (w *LinePrefixWriter) Write(p []byte) (int, error) {
	// If no prefix, just passthrough
	if len(w.prefix) == 0 {
		return w.dst.Write(p)
	}

	consumed := 0

	for i := 0; i < len(p); i++ {
		b := p[i]

		if w.atLineStart {
			if _, err := w.dst.Write(w.prefix); err != nil {
				return consumed, err
			}
			w.atLineStart = false
		}

		if _, err := w.dst.Write([]byte{b}); err != nil {
			return consumed, err
		}

		consumed++

		if b == '\n' {
			w.atLineStart = true
		}
	}

	return consumed, nil
}

func shellCommand(command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd.exe", "/C", command)
	}
	return exec.Command("/bin/sh", "-c", command)
}

func exitCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return 1
}

func main() {
	var (
		command      string
		appendPrefix string
	)

	flag.StringVar(&command, "command", "", "Command string to execute (example: \"ls -lah\" or \"ls\")")
	flag.StringVar(&appendPrefix, "apend-text-line", "", "Prefix text to add at the beginning of each output line")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  %s --command \"ls -lah\"\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s --apend-text-line \"[abc]\" --command \"ls\"\n\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()

	if command == "" {
		fmt.Fprintln(os.Stderr, "Error: --command is required")
		fmt.Fprintln(os.Stderr, "")
		flag.Usage()
		os.Exit(2)
	}

	cmd := shellCommand(command)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: StdoutPipe: %v\n", err)
		os.Exit(1)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: StderrPipe: %v\n", err)
		os.Exit(1)
	}

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: start command: %v\n", err)
		os.Exit(1)
	}

	var outWriter io.Writer = os.Stdout
	var errWriter io.Writer = os.Stderr

	if appendPrefix != "" {
		outWriter = NewLinePrefixWriter(os.Stdout, appendPrefix)
		errWriter = NewLinePrefixWriter(os.Stderr, appendPrefix)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Stream stdout
	go func() {
		defer wg.Done()
		_, _ = io.Copy(outWriter, stdoutPipe)
	}()

	// Stream stderr
	go func() {
		defer wg.Done()
		_, _ = io.Copy(errWriter, stderrPipe)
	}()

	waitErr := cmd.Wait()

	wg.Wait()

	code := exitCodeFromError(waitErr)
	os.Exit(code)
}
