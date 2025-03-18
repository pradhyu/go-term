package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"github.com/pkg/term"
)

// lineWriter wraps an io.Writer and ensures proper line endings
type lineWriter struct {
	w io.Writer
}

func (w *lineWriter) Write(p []byte) (n int, err error) {
	// Convert any lone \n to \r\n
	modified := bytes.ReplaceAll(p, []byte{'\n'}, []byte{'\r', '\n'})
	n, err = w.w.Write(modified)
	if err != nil {
		// Silently handle write errors to prevent them from bubbling up to the user
		return len(p), nil
	}
	return n, nil
}

type Terminal struct {
	term *term.Term
	writer *bufio.Writer
	currentSuggestions []string
	suggestionIndex int
	selectedIndex int
	history []string
	historyIndex int
	historyFile string
	searchMode bool
	searchQuery string
	searchResults []string
	searchIndex int
	currentSuggestion string
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

	// Create terminal instance
	terminal := &Terminal{
		term: t,
		writer: bufio.NewWriter(os.Stdout),
		historyIndex: -1,
		history: []string{},
	}

	// Load history
	if err := terminal.loadHistory(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not load history: %v\n", err)
	}

	return terminal, nil
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
	// Special handling for cd command
	if command == "cd" {
		var dir string
		if len(args) == 0 {
			// No args means cd to home directory
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("could not get home directory: %v", err)
			}
			dir = homeDir
		} else {
			dir = args[0]
			// Handle ~ expansion
			if strings.HasPrefix(dir, "~") {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("could not get home directory: %v", err)
				}
				dir = homeDir + dir[1:]
			}
		}
		// Change directory
		if err := os.Chdir(dir); err != nil {
			return fmt.Errorf("could not change directory: %v", err)
		}
		return nil
	}

	// For all other commands
	// Use the shell to handle environment variables
	shellCmd := command
	for _, arg := range args {
		shellCmd += " " + arg
	}
	
	// Use fish shell to execute the command with environment variable expansion
	cmd := exec.Command("fish", "-c", shellCmd)
	
	// Use our custom writer for stdout
	lw := &lineWriter{w: os.Stdout}
	cmd.Stdout = lw
	cmd.Stderr = lw // Use the same line writer for stderr
	cmd.Stdin = os.Stdin

	// Run the command and handle errors gracefully
	err := cmd.Run()
	if err != nil {
		// Only return the error if it's not a write error
		if !strings.Contains(err.Error(), "write") {
			return err
		}
	}
	return nil
}

// GetPrompt returns a formatted prompt string showing the current directory
func (t *Terminal) GetPrompt() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "> ", err
	}

	// Get the current user's home directory
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}

	// Replace home directory with ~
	if home != "" && strings.HasPrefix(cwd, home) {
		cwd = "~" + cwd[len(home):]
	}

	// Split the path into parts
	parts := strings.Split(cwd, string(filepath.Separator))

	// Start with just the last directory
	result := parts[len(parts)-1]
	maxLen := 20

	// Add parent directories if there's room
	for i := len(parts) - 2; i >= 0; i-- {
		part := parts[i]
		if part == "" {
			part = "/"
		}
		testResult := part + string(filepath.Separator) + result
		if len(testResult) > maxLen {
			// If we can't fit the full parent, add ... and stop
			if i > 0 {
				result = "..." + string(filepath.Separator) + result
			}
			break
		}
		result = testResult
	}

	return result + "> ", nil
}

// ANSI color codes
const (
	greenColor = "\033[32m"
	resetColor = "\033[0m"
	clearToEndLine = "\033[K"
)

// ShowInlineSuggestion displays the current suggestion in green
func (t *Terminal) ShowInlineSuggestion(input string) error {
	// Get unique completions for current input
	completions := t.GetCompletions(input)
	
	// Deduplicate completions
	seen := make(map[string]bool)
	unique := make([]string, 0)
	for _, comp := range completions {
		if !seen[comp] {
			seen[comp] = true
			unique = append(unique, comp)
		}
	}
	completions = unique
	
	if len(completions) == 0 {
		// Clear any existing suggestion
		t.currentSuggestion = ""
		_, err := t.writer.WriteString(clearToEndLine)
		return err
	}

	// Find the best completion (first unique one that starts with current input)
	var suggestion string
	seenSuggestions := make(map[string]bool)
	for _, comp := range completions {
		if strings.HasPrefix(strings.ToLower(comp), strings.ToLower(input)) && !seenSuggestions[comp] {
			seenSuggestions[comp] = true
			suggestion = comp
			break
		}
	}

	if suggestion == "" {
		// Clear any existing suggestion
		t.currentSuggestion = ""
		_, err := t.writer.WriteString(clearToEndLine)
		return err
	}

	// Store the current suggestion
	t.currentSuggestion = suggestion

	// Save cursor position
	_, err := t.writer.WriteString("\033[s")
	if err != nil {
		return err
	}

	// Move to rightmost position (column 60)
	_, err = t.writer.WriteString("\033[60G")
	if err != nil {
		return err
	}

	// Write the full suggestion at rightmost position
	_, err = t.writer.WriteString("[" + greenColor + suggestion + resetColor + "]" + clearToEndLine)
	if err != nil {
		return err
	}

	// Restore cursor position
	_, err = t.writer.WriteString("\033[u")
	if err != nil {
		return err
	}

	// Show the suggestion in green, starting from where the user input ends
	suffixPart := suggestion[len(input):]
	_, err = t.writer.WriteString(greenColor + suffixPart + resetColor + clearToEndLine)
	
	// Move cursor back to end of user input
	if err == nil {
		_, err = t.writer.WriteString(strings.Repeat("\b", len(suffixPart)))
	}

	return t.writer.Flush()
}

// AcceptSuggestion accepts the current suggestion
func (t *Terminal) AcceptSuggestion() string {
	return t.currentSuggestion
}

// SelectNextCompletion moves the selection to the next completion item
func (t *Terminal) SelectNextCompletion() {
	if len(t.currentSuggestions) > 0 {
		t.selectedIndex = (t.selectedIndex + 1) % len(t.currentSuggestions)
		t.ShowCompletions()
	}
}

// SelectPreviousCompletion moves the selection to the previous completion item
func (t *Terminal) SelectPreviousCompletion() {
	if len(t.currentSuggestions) > 0 {
		t.selectedIndex = (t.selectedIndex - 1 + len(t.currentSuggestions)) % len(t.currentSuggestions)
		t.ShowCompletions()
	}
}

// GetSelectedCompletion returns the currently selected completion
func (t *Terminal) GetSelectedCompletion() string {
	if len(t.currentSuggestions) > 0 && t.selectedIndex >= 0 && t.selectedIndex < len(t.currentSuggestions) {
		suggestion := t.currentSuggestions[t.selectedIndex]
		// Remove any prefix (HIST: or CMD:)
		if strings.Contains(suggestion, ": ") {
			parts := strings.SplitN(suggestion, ": ", 2)
			if len(parts) == 2 {
				return parts[1]
			}
		}
		return suggestion
	}
	return ""
}

// GetCompletions returns possible completions for the current input
func (t *Terminal) GetCompletions(input string) []string {
	// If input is empty, show unique history items
	if input == "" {
		// Get unique recent history items (up to 3)
		var uniqueHistory []string
		var seenCommands = make(map[string]bool)
		for i := len(t.history) - 1; i >= 0 && len(uniqueHistory) < 3; i-- {
			cmd := t.history[i]
			if !seenCommands[cmd] {
				seenCommands[cmd] = true
				uniqueHistory = append(uniqueHistory, cmd)
			}
		}

		// Add prefix to unique history matches
		var historyItems = make([]string, len(uniqueHistory))
		for i, cmd := range uniqueHistory {
			historyItems[i] = "HIST: " + cmd
		}
		return historyItems
	}

	// Check if input matches any unique history items (up to 3)
	seen := make(map[string]bool)
	historyMatches := []string{}
	for i := len(t.history) - 1; i >= 0 && len(historyMatches) < 3; i-- {
		if strings.HasPrefix(strings.ToLower(t.history[i]), strings.ToLower(input)) && !seen[t.history[i]] {
			seen[t.history[i]] = true
			historyMatches = append(historyMatches, t.history[i])
		}
	}

	// Split input into command and current argument
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return historyMatches
	}

	// If we're completing the command itself
	if len(parts) == 1 && !strings.Contains(input, " ") {
		// Get PATH directories
		pathDirs := strings.Split(os.Getenv("PATH"), ":")
		
		// Add built-in commands
		builtins := []string{"cd", "clear", "exit", "help", "quit"}
		completions := make(map[string]bool)
		
		// Add matching built-ins
		for _, cmd := range builtins {
			if strings.HasPrefix(cmd, parts[0]) {
				completions[cmd] = true
			}
		}

		// Search PATH for executables
		for _, dir := range pathDirs {
			files, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, file := range files {
				name := file.Name()
				if strings.HasPrefix(name, parts[0]) {
					completions[name] = true
				}
			}
		}

		// Convert map to sorted slice and add CMD: prefix
		result := make([]string, 0, len(completions))
		for cmd := range completions {
			result = append(result, "CMD: "+cmd)
		}
		sort.Strings(result)
		
		// Get unique history matches
		var uniqueHistory []string
		var seenHistory = make(map[string]bool)
		for i := len(t.history) - 1; i >= 0 && len(uniqueHistory) < 3; i-- {
			cmd := t.history[i]
			if strings.HasPrefix(strings.ToLower(cmd), strings.ToLower(parts[0])) && !seenHistory[cmd] {
				seenHistory[cmd] = true
				uniqueHistory = append(uniqueHistory, cmd)
			}
		}

		// Add prefix to unique history matches
		var historyMatches = make([]string, len(uniqueHistory))
		for i, cmd := range uniqueHistory {
			historyMatches[i] = "HIST: " + cmd
		}
		
		// Combine history with unique commands (limit to 3 commands)
		if len(result) > 3 {
			result = result[:3]
		}
		if len(historyMatches) > 0 {
			combined := append(historyMatches, result...)
			return combined
		}
		
		// If no history matches, limit regular completions to 6
		if len(result) > 6 {
			result = result[:6]
		}
		return result
	}

	// If we're completing a path argument
	if len(parts) > 0 {
		currentArg := ""
		if strings.HasSuffix(input, " ") {
			currentArg = ""
		} else {
			currentArg = parts[len(parts)-1]
		}

		// Expand ~ in path
		if strings.HasPrefix(currentArg, "~") {
			homeDir, err := os.UserHomeDir()
			if err == nil {
				currentArg = homeDir + currentArg[1:]
			}
		}

		// Get the directory to search in
		searchDir := "."
		searchPrefix := ""
		if currentArg != "" {
			searchDir = filepath.Dir(currentArg)
			searchPrefix = filepath.Base(currentArg)
		}

		// Read directory contents
		files, err := os.ReadDir(searchDir)
		if err != nil {
			return nil
		}

		// Filter and format unique completions
		seen := make(map[string]bool)
		var completions []string
		for _, file := range files {
			name := file.Name()
			if strings.HasPrefix(name, searchPrefix) && !seen[name] {
				seen[name] = true
				if file.IsDir() {
					name += "/"
				}
				completions = append(completions, "CMD: "+name)
			}
		}

		sort.Strings(completions)
		
		// Check if any unique history items match the current path
		var uniqueHistory []string
		var seenPaths = make(map[string]bool)
		for i := len(t.history) - 1; i >= 0 && len(uniqueHistory) < 3; i-- {
			cmd := t.history[i]
			if strings.Contains(cmd, currentArg) && !seenPaths[cmd] {
				seenPaths[cmd] = true
				uniqueHistory = append(uniqueHistory, cmd)
			}
		}

		// Add prefix to unique history matches
		var historyMatches = make([]string, len(uniqueHistory))
		for i, cmd := range uniqueHistory {
			historyMatches[i] = "HIST: " + cmd
		}

		// Limit completions to 3 if we have history matches
		if len(historyMatches) > 0 && len(completions) > 3 {
			completions = completions[:3]
		} else if len(completions) > 6 { // Otherwise limit to 6
			completions = completions[:6]
		}
		
		// Combine history with file completions
		if len(historyMatches) > 0 {
			combined := append(historyMatches, completions...)
			return combined
		}
		
		return completions
	}

	return nil
}

// ANSI color codes
const (
	YellowBg = "\033[43m" // Yellow background
	BlackFg  = "\033[30m" // Black foreground
	GreenBg  = "\033[42m" // Green background
	BlueBg   = "\033[44m" // Blue background for history items
	Reset    = "\033[0m"  // Reset formatting
)

// ClearCompletions clears the dropdown completion menu
func (t *Terminal) ClearCompletions() error {
	// Save cursor position
	_, err := t.writer.WriteString("\033[s")
	if err != nil {
		return err
	}

	// Move cursor down one line
	_, err = t.writer.WriteString("\r\n")
	if err != nil {
		return err
	}

	// Clear several lines below (enough to cover the dropdown)
	for i := 0; i < 12; i++ {
		_, err = t.writer.WriteString("\033[K\r\n")
		if err != nil {
			return err
		}
	}

	// Restore cursor position
	_, err = t.writer.WriteString("\033[u")
	if err != nil {
		return err
	}

	return t.writer.Flush()
}

// ShowCompletions displays the current completion suggestions in a dropdown with yellow background
func (t *Terminal) ShowCompletions() error {
	// First clear any existing dropdown
	if err := t.ClearCompletions(); err != nil {
		return err
	}

	if len(t.currentSuggestions) == 0 {
		return nil
	}

	// Get terminal width
	cmd := exec.Command("tput", "cols")
	output, err := cmd.Output()
	if err != nil {
		return err
	}
	termWidth, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		termWidth = 80 // fallback width
	}

	// Save cursor position
	_, err = t.writer.WriteString("\033[s")
	if err != nil {
		return err
	}

	// Draw dropdown box with yellow background
	maxWidth := 25
	maxItems := 6 // Maximum number of items to show in dropdown

	// Limit the number of suggestions shown
	shownSuggestions := t.currentSuggestions
	if len(shownSuggestions) > maxItems {
		shownSuggestions = shownSuggestions[:maxItems]
	}

	// Calculate rightmost position
	rightPos := termWidth - maxWidth - 2 // -2 for box borders

	// Draw top border at rightmost position
	_, err = t.writer.WriteString(fmt.Sprintf("\033[%dG", rightPos))
	if err != nil {
		return err
	}
	_, err = t.writer.WriteString("┌" + strings.Repeat("─", maxWidth) + "┐\r\n")
	if err != nil {
		return err
	}

	// First suggestion will be highlighted in green

	// Draw suggestions
	for i, suggestion := range shownSuggestions {
		// Move to rightmost position
		_, err = t.writer.WriteString(fmt.Sprintf("\033[%dG", rightPos))
		if err != nil {
			return err
		}

		// Pad suggestion to fixed width
		padded := suggestion
		if len(padded) > maxWidth-2 {
			padded = padded[:maxWidth-5] + "..."
		} else {
			padded = padded + strings.Repeat(" ", maxWidth-2-len(padded))
		}

		// Determine background color based on type and position
		background := YellowBg
		
		// Use green for selected item
		if i == t.selectedIndex {
			background = GreenBg
		}
		
		// Use blue background for history items
		if strings.HasPrefix(suggestion, "HIST: ") {
			if i == t.selectedIndex {
				background = "\033[44m" // Bright blue for selected history
			} else {
				background = BlueBg
			}
		}

		// Write with colored background and black text
		_, err = t.writer.WriteString("│" + background + BlackFg + padded + Reset + "│\r\n")
		if err != nil {
			return err
		}
	}

	// Move to rightmost position and draw bottom border
	_, err = t.writer.WriteString(fmt.Sprintf("\033[%dG", rightPos))
	if err != nil {
		return err
	}
	_, err = t.writer.WriteString("└" + strings.Repeat("─", maxWidth) + "┘")
	if err != nil {
		return err
	}

	// Restore cursor position
	_, err = t.writer.WriteString("\033[u")
	if err != nil {
		return err
	}

	return t.writer.Flush()
}

// AddToHistory adds a command to history and saves it
func (t *Terminal) AddToHistory(cmd string) error {
	// Don't add empty commands or duplicates of the last command
	if cmd == "" || (len(t.history) > 0 && t.history[len(t.history)-1] == cmd) {
		return nil
	}

	// Add to memory
	t.history = append(t.history, cmd)

	// Trim history to last 1000 commands
	if len(t.history) > 1000 {
		t.history = t.history[len(t.history)-1000:]
	}

	// Save to file
	return t.saveHistory()
}

// GetPreviousHistory moves back in history
func (t *Terminal) GetPreviousHistory() string {
	if len(t.history) == 0 {
		return ""
	}

	// First time pressing up arrow
	if t.historyIndex == -1 {
		t.historyIndex = len(t.history) - 1
	} else if t.historyIndex > 0 {
		// Move back in history
		t.historyIndex--
	}

	return t.history[t.historyIndex]
}

// GetNextHistory moves forward in history
func (t *Terminal) GetNextHistory() string {
	if t.historyIndex == -1 || len(t.history) == 0 {
		return ""
	}

	if t.historyIndex < len(t.history)-1 {
		// Move forward in history
		t.historyIndex++
		return t.history[t.historyIndex]
	} else {
		// Reached the end of history
		t.historyIndex = -1
		return ""
	}
}

// ResetHistoryIndex resets the history navigation index
func (t *Terminal) ResetHistoryIndex() {
	t.historyIndex = -1
}

// loadHistory loads command history from file
func (t *Terminal) loadHistory() error {
	// Create history directory if it doesn't exist
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not get home directory: %v", err)
	}

	// Set history file path
	t.historyFile = filepath.Join(homeDir, ".go_term_history")

	// Try to read existing history file
	data, err := os.ReadFile(t.historyFile)
	if err != nil {
		if os.IsNotExist(err) {
			// Create empty history file
			if err := os.WriteFile(t.historyFile, []byte{}, 0600); err != nil {
				return fmt.Errorf("could not create history file: %v", err)
			}
			// Initialize empty history
			t.history = []string{}
			return nil
		}
		return err
	}

	// Split by newlines and filter empty lines
	t.history = []string{}
	lines := strings.Split(string(data), "\n")
	for _, cmd := range lines {
		if cmd != "" {
			t.history = append(t.history, cmd)
		}
	}

	return nil
}

// saveHistory saves command history to file
func (t *Terminal) saveHistory() error {
	// Make sure we have a valid history file path
	if t.historyFile == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("could not get home directory: %v", err)
		}
		t.historyFile = filepath.Join(homeDir, ".go_term_history")
	}

	// Join history with newlines and save
	data := strings.Join(t.history, "\n")
	return os.WriteFile(t.historyFile, []byte(data), 0600)
}

// StartHistorySearch enters history search mode
func (t *Terminal) StartHistorySearch() {
	t.searchMode = true
	t.searchQuery = ""
	t.searchResults = nil
	t.searchIndex = -1
}

// ExitHistorySearch exits history search mode
func (t *Terminal) ExitHistorySearch() {
	t.searchMode = false
	t.searchQuery = ""
	t.searchResults = nil
	t.searchIndex = -1
}

// IsInSearchMode returns whether we're in history search mode
func (t *Terminal) IsInSearchMode() bool {
	return t.searchMode
}

// UpdateHistorySearch updates the search results based on the current query
func (t *Terminal) UpdateHistorySearch(query string) []string {
	t.searchQuery = query
	t.searchResults = nil
	t.searchIndex = -1

	// Search through history in reverse order
	for i := len(t.history) - 1; i >= 0; i-- {
		if strings.Contains(strings.ToLower(t.history[i]), strings.ToLower(query)) {
			t.searchResults = append(t.searchResults, t.history[i])
		}
	}

	if len(t.searchResults) > 0 {
		t.searchIndex = 0
		return []string{t.searchResults[0]}
	}

	return nil
}

// GetNextSearchResult moves to the next search result
func (t *Terminal) GetNextSearchResult() string {
	if len(t.searchResults) == 0 {
		return ""
	}

	t.searchIndex = (t.searchIndex + 1) % len(t.searchResults)
	return t.searchResults[t.searchIndex]
}

// GetPreviousSearchResult moves to the previous search result
func (t *Terminal) GetPreviousSearchResult() string {
	if len(t.searchResults) == 0 {
		return ""
	}

	t.searchIndex--
	if t.searchIndex < 0 {
		t.searchIndex = len(t.searchResults) - 1
	}
	return t.searchResults[t.searchIndex]
}

// GetSearchPrompt returns the search prompt with current query
func (t *Terminal) GetSearchPrompt() string {
	return fmt.Sprintf("(reverse-i-search)`%s': ", t.searchQuery)
}

// Clear clears the terminal screen and resets cursor position
func (t *Terminal) Clear() error {
	_, err := t.writer.WriteString("\033[2J\033[H")
	if err != nil {
		return err
	}
	return t.writer.Flush()
}
