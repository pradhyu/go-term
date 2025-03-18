package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func main() {
	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	term, err := NewTerminal()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating terminal: %v\n", err)
		os.Exit(1)
	}

	// Handle Ctrl+C gracefully
	go func() {
		<-sigChan
		fmt.Print("\n") // Move to new line
		term.Close()
		os.Exit(0)
	}()

	defer term.Close()

	term.Clear()
	term.WriteLine("Go Terminal REPL (type 'help' for commands, 'exit' to quit, or press Ctrl+C)")
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
			if len(term.currentSuggestions) > 0 {
				// If we have suggestions, accept the selected one
				if selected := term.GetSelectedCompletion(); selected != "" {
					// Clear current input
					clearLine(prompt + cmdBuffer.String())

					// If we're completing a command, add a space
					if !strings.Contains(selected, " ") {
						selected += " "
					}

					// Update buffer and display
					cmdBuffer.Reset()
					cmdBuffer.WriteString(selected)
					fmt.Print(prompt + selected)

					// Clear completions but get new ones if needed
					term.ClearCompletions()

					// Get new completions based on the accepted selection
					currentInput := cmdBuffer.String()
					term.currentSuggestions = term.GetCompletions(currentInput)

					// Show new completions if available
					if len(term.currentSuggestions) > 0 {
						term.selectedIndex = 0 // Reset selection to first item
						term.ShowCompletions()
					}
				}
			} else {
				// Get current input
				currentInput := cmdBuffer.String()

				// Get completions
				term.currentSuggestions = term.GetCompletions(currentInput)

				if len(term.currentSuggestions) > 0 {
					// Show all completions in dropdown menu with first item selected
					term.selectedIndex = 0
					term.ShowCompletions()

					// Reprint prompt and current input
					fmt.Print(prompt + currentInput)

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
		case 27: // ESC sequence
			// Read [ character
			if ch, err = term.ReadChar(); err != nil || ch != 91 {
				continue
			}

			// Read the next character
			if ch, err = term.ReadChar(); err != nil {
				continue
			}

			// Check for Ctrl+Up and Ctrl+Down key combinations
			ctrlKey := false
			ctrlUpDown := false

			// Check for Ubuntu terminal's Ctrl+Up (ESC [ 5 A) and Ctrl+Down (ESC [ 5 B)
			if ch == 53 { // '5' - this is the Ubuntu terminal's sequence for Ctrl+arrow
				// Try to read the next character (A for Up, B for Down)
				if finalCh, err := term.ReadChar(); err == nil {
					if finalCh == 65 { // 'A' for Up
						// Handle Ctrl+Up
						ctrlKey = true
						ctrlUpDown = true

						// Get completions if not already visible
						if len(term.currentSuggestions) == 0 {
							currentInput := cmdBuffer.String()
							term.currentSuggestions = term.GetCompletions(currentInput)
							if len(term.currentSuggestions) > 0 {
								term.selectedIndex = 0
								term.ShowCompletions()
							}
						}

						// Navigate up in the completion menu
						if len(term.currentSuggestions) > 0 {
							term.SelectPreviousCompletion()
						}
						continue
					} else if finalCh == 66 { // 'B' for Down
						// Handle Ctrl+Down
						ctrlKey = true
						ctrlUpDown = true

						// Get completions if not already visible
						if len(term.currentSuggestions) == 0 {
							currentInput := cmdBuffer.String()
							term.currentSuggestions = term.GetCompletions(currentInput)
							if len(term.currentSuggestions) > 0 {
								term.selectedIndex = 0
								term.ShowCompletions()
							}
						}

						// Navigate down in the completion menu
						if len(term.currentSuggestions) > 0 {
							term.SelectNextCompletion()
						}
						continue
					}
					ch = finalCh
				}
			}

			// Also handle standard Ctrl+Up/Down sequence (ESC [ 1 ; 5 A/B)
			if ch == 49 && !ctrlUpDown { // '1'
				// Try to read the next characters in the sequence
				var nextCh, modCh, finalCh byte
				if nextCh, err = term.ReadChar(); err == nil && nextCh == 59 { // ';'
					if modCh, err = term.ReadChar(); err == nil && modCh == 53 { // '5' for Ctrl
						if finalCh, err = term.ReadChar(); err == nil {
							if finalCh == 65 { // 'A' for Up
								// Handle Ctrl+Up
								ctrlKey = true
								ctrlUpDown = true

								// Get completions if not already visible
								if len(term.currentSuggestions) == 0 {
									currentInput := cmdBuffer.String()
									term.currentSuggestions = term.GetCompletions(currentInput)
									if len(term.currentSuggestions) > 0 {
										term.selectedIndex = 0
										term.ShowCompletions()
									}
								}

								// Navigate up in the completion menu
								if len(term.currentSuggestions) > 0 {
									term.SelectPreviousCompletion()
								}
								continue
							} else if finalCh == 66 { // 'B' for Down
								// Handle Ctrl+Down
								ctrlKey = true
								ctrlUpDown = true

								// Get completions if not already visible
								if len(term.currentSuggestions) == 0 {
									currentInput := cmdBuffer.String()
									term.currentSuggestions = term.GetCompletions(currentInput)
									if len(term.currentSuggestions) > 0 {
										term.selectedIndex = 0
										term.ShowCompletions()
									}
								}

								// Navigate down in the completion menu
								if len(term.currentSuggestions) > 0 {
									term.SelectNextCompletion()
								}
								continue
							}
							ch = finalCh
						}
					}
				}
			}

			// If we've already handled a Ctrl+Up/Down, skip the rest
			if ctrlUpDown {
				continue
			}

			switch ch {
			case 65: // Up arrow
				if ctrlKey || len(term.currentSuggestions) > 0 {
					// For Ctrl+Up or if completion menu is visible, navigate completion menu
					if len(term.currentSuggestions) == 0 {
						// If no completions visible yet, get them first
						currentInput := cmdBuffer.String()
						term.currentSuggestions = term.GetCompletions(currentInput)
						if len(term.currentSuggestions) > 0 {
							term.selectedIndex = 0
							term.ShowCompletions()
						}
					}

					if len(term.currentSuggestions) > 0 {
						term.SelectPreviousCompletion()
					}
				} else {
					// Regular up arrow - history navigation
					handleUpArrow()
				}
			case 66: // Down arrow
				if ctrlKey || len(term.currentSuggestions) > 0 {
					// For Ctrl+Down or if completion menu is visible, navigate completion menu
					if len(term.currentSuggestions) == 0 {
						// If no completions visible yet, get them first
						currentInput := cmdBuffer.String()
						term.currentSuggestions = term.GetCompletions(currentInput)
						if len(term.currentSuggestions) > 0 {
							term.selectedIndex = 0
							term.ShowCompletions()
						}
					}

					if len(term.currentSuggestions) > 0 {
						term.SelectNextCompletion()
					}
				} else {
					// Regular down arrow - history navigation
					handleDownArrow()
				}
			case 67: // Right arrow
				if len(term.currentSuggestions) > 0 {
					// Accept selected completion
					if selected := term.GetSelectedCompletion(); selected != "" {
						clearLine(prompt + cmdBuffer.String())
						cmdBuffer.Reset()

						// If we're completing a command, add a space
						if !strings.Contains(selected, " ") {
							selected += " "
						}

						cmdBuffer.WriteString(selected)
						fmt.Print(prompt + selected)

						// Clear completions but get new ones if needed
						term.ClearCompletions()

						// Get new completions based on the accepted selection
						currentInput := cmdBuffer.String()
						term.currentSuggestions = term.GetCompletions(currentInput)

						// Show new completions if available
						if len(term.currentSuggestions) > 0 {
							term.selectedIndex = 0 // Reset selection to first item
							term.ShowCompletions()
						}
					}
				}
			}
			continue

		default:
			// Check for special key sequences that might be embedded in the input
			inputStr := cmdBuffer.String()
			if len(inputStr) >= 2 && (inputStr[len(inputStr)-2:] == "5A" || inputStr[len(inputStr)-2:] == "5B") {
				// Get the last two characters
				lastTwo := inputStr[len(inputStr)-2:]

				// Remove the sequence from the buffer
				cmdBuffer.Reset()
				cmdBuffer.WriteString(inputStr[:len(inputStr)-2])

				// Clear the characters from the screen
				fmt.Print("\b\b  \b\b")

				// Handle Ctrl+Up/Down
				if lastTwo == "5A" { // Ctrl+Up
					// Get completions if not already visible
					if len(term.currentSuggestions) == 0 {
						currentInput := cmdBuffer.String()
						term.currentSuggestions = term.GetCompletions(currentInput)
						if len(term.currentSuggestions) > 0 {
							term.selectedIndex = 0
							term.ShowCompletions()
						}
					}

					// Navigate up in the completion menu
					if len(term.currentSuggestions) > 0 {
						term.SelectPreviousCompletion()
					}
				} else if lastTwo == "5B" { // Ctrl+Down
					// Get completions if not already visible
					if len(term.currentSuggestions) == 0 {
						currentInput := cmdBuffer.String()
						term.currentSuggestions = term.GetCompletions(currentInput)
						if len(term.currentSuggestions) > 0 {
							term.selectedIndex = 0
							term.ShowCompletions()
						}
					}

					// Navigate down in the completion menu
					if len(term.currentSuggestions) > 0 {
						term.SelectNextCompletion()
					}
				}
				continue
			}

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
					term.selectedIndex = 0 // Reset selection to first item
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
