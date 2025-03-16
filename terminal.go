package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"github.com/pkg/term"
)

// lineWriter wraps an io.Writer and ensures proper line endings
type lineWriter struct {
	w io.Writer
}

func (w *lineWriter) Write(p []byte) (n int, err error) {
	// Convert any lone \n to \r\n
	modified := bytes.ReplaceAll(p, []byte{'\n'}, []byte{'\r', '\n'})
	return w.w.Write(modified)
}

type Terminal struct {
	term *term.Term
	writer *bufio.Writer
}

// NewTerminal creates a new terminal wrapper
func NewTerminal() (*Terminal, error) {
	t, err := term.Open("/dev/tty")
	if err != nil {
		return nil, fmt.Errorf("failed to open terminal: %v", err)
	}

	// Set raw mode
	err = term.RawMode(t)
	if err != nil {
		t.Close()
		return nil, fmt.Errorf("failed to set raw mode: %v", err)
	}

	return &Terminal{
		term: t,
		writer: bufio.NewWriter(os.Stdout),
	}, nil
}

// Close closes the terminal
func (t *Terminal) Close() error {
	return t.term.Close()
}

// ReadChar reads a single character from the terminal
func (t *Terminal) ReadChar() (byte, error) {
	buf := make([]byte, 1)
	_, err := t.term.Read(buf)
	if err != nil {
		return 0, err
	}
	return buf[0], nil
}

// Write writes data to the terminal
func (t *Terminal) Write(data []byte) (int, error) {
	return t.term.Write(data)
}

// WriteLine writes a line to the terminal with proper line ending
func (t *Terminal) WriteLine(s string) error {
	// Write the content with both carriage return and newline
	_, err := t.writer.WriteString(s)
	if err != nil {
		return err
	}
	_, err = t.writer.WriteString("\r\n")
	if err != nil {
		return err
	}
	return t.writer.Flush()
}

// ExecuteCommand executes a shell command
func (t *Terminal) ExecuteCommand(command string, args ...string) error {
	cmd := exec.Command(command, args...)
	
	// Use our custom writer for stdout
	lw := &lineWriter{w: os.Stdout}
	cmd.Stdout = lw
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

// Clear clears the terminal screen and resets cursor position
func (t *Terminal) Clear() error {
	_, err := t.writer.WriteString("\033[2J\033[H")
	if err != nil {
		return err
	}
	return t.writer.Flush()
}
