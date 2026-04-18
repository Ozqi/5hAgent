package cli

import (
	"fmt"
	"strings"
)

// PrintUserInput prints user input with a prefix
func PrintUserInput(text string) {
	fmt.Printf("User: %s\n", text)
}

// PrintAssistantChunk prints AI response chunks (streaming style, no newline)
func PrintAssistantChunk(text string) {
	fmt.Print(text)
}

// PrintToolCall prints tool invocation information
func PrintToolCall(name, input string) {
	fmt.Printf("[Tool] %s(%s)\n", name, input)
}

// PrintToolResult prints tool execution result with indentation
func PrintToolResult(result string) {
	lines := strings.Split(result, "\n")
	for _, line := range lines {
		if line != "" {
			fmt.Printf("  Result: %s\n", line)
		}
	}
}

// PrintError prints error messages in red (using ANSI color codes)
func PrintError(err error) {
	fmt.Printf("\033[31mError: %v\033[0m\n", err)
}
