package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	term, err := NewTerminal()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating terminal: %v\n", err)
		os.Exit(1)
	}
	defer term.Close()

	term.Clear()
	term.WriteLine("Go Terminal REPL (type 'help' for commands, 'exit' to quit)")
	term.WriteLine("")

	var cmdBuffer strings.Builder

	// Show initial prompt
	fmt.Print("> ")

	for {
		ch, err := term.ReadChar()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
			break
		}

		switch ch {
		case '\r', '\n': // Enter key
			cmd := cmdBuffer.String()
			term.WriteLine("") // New line after command

			if cmd != "" {
				// Handle built-in commands
				switch cmd {
				case "exit", "quit":
					return
				case "clear":
					term.Clear()
				case "help":
					term.WriteLine("Available commands:")
					term.WriteLine("  clear  - Clear the screen")
					term.WriteLine("  exit   - Exit the terminal")
					term.WriteLine("  help   - Show this help message")
					term.WriteLine("  quit   - Same as exit")
					term.WriteLine("")
					term.WriteLine("Any other input will be executed as a shell command")
					term.WriteLine("")
				default:
					// Execute as shell command
					parts := strings.Fields(cmd)
					if len(parts) > 0 {
						if err := term.ExecuteCommand(parts[0], parts[1:]...); err != nil {
							term.WriteLine(fmt.Sprintf("Error: %v", err))
						}
					}
				}
			}
			cmdBuffer.Reset()
			fmt.Print("> ")
		case 127, 8: // Backspace
			if cmdBuffer.Len() > 0 {
				// Remove last character from buffer and terminal
				s := cmdBuffer.String()
				cmdBuffer.Reset()
				cmdBuffer.WriteString(s[:len(s)-1])
				// Move cursor back and clear character
				fmt.Print("\b \b")
			}
		default:
			cmdBuffer.WriteByte(ch)
			fmt.Print(string(ch)) // Echo character
		}
	}
}
