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
	prompt, err := term.GetPrompt()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting prompt: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(prompt)

	// Function to clear the current line
	clearLine := func(text string) {
		for i := 0; i < len(text); i++ {
			fmt.Print("\b \b")
		}
	}

	// Function to handle up arrow key (previous history)
	handleUpArrow := func() {
		// Clear current line
		clearLine(prompt + cmdBuffer.String())

		// Get previous command from history
		if cmd := term.GetPreviousHistory(); cmd != "" {
			cmdBuffer.Reset()
			cmdBuffer.WriteString(cmd)
			fmt.Print(prompt + cmd)

			// Show inline suggestion
			if err := term.ShowInlineSuggestion(cmd); err != nil {
				fmt.Fprintf(os.Stderr, "Error showing suggestion: %v\n", err)
			}
		}
	}

	// Function to handle down arrow key (next history)
	handleDownArrow := func() {
		// Clear current line
		clearLine(prompt + cmdBuffer.String())

		// Get next command from history
		cmd := term.GetNextHistory()
		cmdBuffer.Reset()
		cmdBuffer.WriteString(cmd)
		fmt.Print(prompt + cmd)

		// Show inline suggestion
		if err := term.ShowInlineSuggestion(cmd); err != nil {
			fmt.Fprintf(os.Stderr, "Error showing suggestion: %v\n", err)
		}
	}

	// Function to refresh the prompt and input
	refreshLine := func(prompt, text string) {
		// Clear everything
		clearLine(prompt + text)

		// Print new prompt and text
		fmt.Print(prompt + text)
	}

	for {
		ch, err := term.ReadChar()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
			break
		}

		// Handle Ctrl+R for search mode
		if ch == 18 { // Ctrl+R
			if !term.IsInSearchMode() {
				// Enter search mode
				term.StartHistorySearch()
				// Clear current line and show search prompt
				clearLine(prompt + cmdBuffer.String())
				fmt.Print(term.GetSearchPrompt())
			}
			continue
		}

		// Handle input in search mode
		if term.IsInSearchMode() {
			switch ch {
			case 27: // Escape
				// Exit search mode
				term.ExitHistorySearch()
				refreshLine(prompt, cmdBuffer.String())

			case '\r', '\n': // Enter
				// Exit search mode and keep the result
				term.ExitHistorySearch()
				term.WriteLine("") // New line after command
				cmd := cmdBuffer.String()

				// Reset history index when executing a command
				term.ResetHistoryIndex()

				if cmd != "" {
					// Add command to history
					if err := term.AddToHistory(cmd); err != nil {
						term.WriteLine(fmt.Sprintf("Error saving history: %v", err))
					}

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
				fmt.Print(prompt)

			case 127, 8: // Backspace
				if len(term.searchQuery) > 0 {
					// Update search query
					newQuery := term.searchQuery[:len(term.searchQuery)-1]
					results := term.UpdateHistorySearch(newQuery)
					
					// Clear current line
					clearLine(term.GetSearchPrompt() + cmdBuffer.String())
					
					// Update command buffer if we have results
					if len(results) > 0 {
						cmdBuffer.Reset()
						cmdBuffer.WriteString(results[0])
					}
					
					// Show new prompt and command
					fmt.Print(term.GetSearchPrompt() + cmdBuffer.String())
				}

			case '\t': // Tab - cycle through results
				if result := term.GetNextSearchResult(); result != "" {
					clearLine(term.GetSearchPrompt() + cmdBuffer.String())
					cmdBuffer.Reset()
					cmdBuffer.WriteString(result)
					fmt.Print(term.GetSearchPrompt() + cmdBuffer.String())
				}

			default:
				if ch >= 32 && ch < 127 { // Printable characters
					// Update search query
					newQuery := term.searchQuery + string(ch)
					results := term.UpdateHistorySearch(newQuery)

					// Clear current line
					clearLine(term.GetSearchPrompt() + cmdBuffer.String())

					// Update command buffer if we have results
					if len(results) > 0 {
						cmdBuffer.Reset()
						cmdBuffer.WriteString(results[0])
					}

					// Show new prompt and command
					fmt.Print(term.GetSearchPrompt() + cmdBuffer.String())
				}
			}
			continue
		}

		// Handle escape sequences (arrow keys)
		if ch == 27 {
			// Read next character
			ch, err = term.ReadChar()
			if err != nil {
				continue
			}

			// Check for special keys
			if ch == 'A' { // Up arrow in some terminals
				handleUpArrow()
				continue
			} else if ch == 'B' { // Down arrow in some terminals
				handleDownArrow()
				continue
			}

			// Handle standard escape sequences
			if ch == 'O' || ch == '[' {
				// Read actual code
				ch, err = term.ReadChar()
				if err != nil {
					continue
				}
			}

			switch ch {
			case 'A': // Up arrow
				handleUpArrow()

			case 'B': // Down arrow
				handleDownArrow()
			}
			continue
		}

		switch ch {
		case '\t': // Tab key
			// Clear any dropdown completion menu first
			term.ClearCompletions()
			
			// Accept current suggestion if there is one
			if suggestion := term.AcceptSuggestion(); suggestion != "" {
				// Clear current input
				currentInput := cmdBuffer.String()
				for i := 0; i < len(currentInput); i++ {
					fmt.Print("\b \b")
				}

				// If we're completing a command, add a space
				if !strings.Contains(suggestion, " ") {
					suggestion += " "
				}

				// Update buffer and display
				cmdBuffer.Reset()
				cmdBuffer.WriteString(suggestion)
				fmt.Print(suggestion)
			} else {
				// Get current input
				currentInput := cmdBuffer.String()
				
				// Clear any existing dropdown first
				term.ClearCompletions()
				
				// Get completions
				term.currentSuggestions = term.GetCompletions(currentInput)
				
				if len(term.currentSuggestions) > 0 {
					// Show all completions in dropdown menu
					term.ShowCompletions()
					
					// Reprint prompt and current input
					prompt, _ := term.GetPrompt()
					fmt.Print(prompt, currentInput)

					// Show inline suggestion again
					if err := term.ShowInlineSuggestion(currentInput); err != nil {
						fmt.Fprintf(os.Stderr, "Error showing suggestion: %v\n", err)
					}
				}
			}

		case '\r', '\n': // Enter key
			// Clear any dropdown completion menu
			term.ClearCompletions()
			
			cmd := cmdBuffer.String()
			term.WriteLine("") // New line after command

			// Reset history index when executing a command
			term.ResetHistoryIndex()

			if cmd != "" {
				// Add command to history
				if err := term.AddToHistory(cmd); err != nil {
					term.WriteLine(fmt.Sprintf("Error saving history: %v", err))
				}

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
			// Update prompt in case directory changed
			prompt, err := term.GetPrompt()
			if err != nil {
				term.WriteLine(fmt.Sprintf("Error getting prompt: %v", err))
				prompt = "> "
			}
			fmt.Print(prompt)
		case 127, 8: // Backspace
			// Clear any dropdown completion menu
			term.ClearCompletions()
			
			if cmdBuffer.Len() > 0 {
				// Remove last character from buffer and terminal
				s := cmdBuffer.String()
				cmdBuffer.Reset()
				cmdBuffer.WriteString(s[:len(s)-1])
				// Move cursor back and clear character
				fmt.Print("\b \b")

				// Update inline suggestion
				if err := term.ShowInlineSuggestion(cmdBuffer.String()); err != nil {
					fmt.Fprintf(os.Stderr, "Error showing suggestion: %v\n", err)
				}
			}
		default:
			if ch >= 32 && ch < 127 { // Printable characters
				// Echo character
				fmt.Printf("%c", ch)
				cmdBuffer.WriteByte(ch)

				// Get current input for completions
				currentInput := cmdBuffer.String()
				
				// Clear any existing dropdown first
				term.ClearCompletions()
				
				// Get completions for dropdown menu
				term.currentSuggestions = term.GetCompletions(currentInput)
				
				// Show dropdown completion menu with yellow background if we have suggestions
				if len(term.currentSuggestions) > 0 {
					term.ShowCompletions()
				}

				// Show inline suggestion
				if err := term.ShowInlineSuggestion(cmdBuffer.String()); err != nil {
					fmt.Fprintf(os.Stderr, "Error showing suggestion: %v\n", err)
				}
			}
		}
	}
}
