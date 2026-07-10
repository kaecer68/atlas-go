package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// AskYN prompts the user with a yes/no question on stderr (so stdout can
// still be piped). Returns the default value on EOF or empty input.
func AskYN(question string, def bool) bool {
	r := bufio.NewReader(os.Stdin)
	hint := "[Y/n]"
	if !def {
		hint = "[y/N]"
	}
	for {
		fmt.Fprintf(os.Stderr, "%s %s ", question, hint)
		line, err := r.ReadString('\n')
		if err != nil && err != io.EOF {
			return def
		}
		ans := strings.TrimSpace(strings.ToLower(line))
		if ans == "" {
			return def
		}
		if ans == "y" || ans == "yes" {
			return true
		}
		if ans == "n" || ans == "no" {
			return false
		}
		// Loop on invalid input.
	}
}

// AskChoice prompts the user to pick one of the options. Returns the
// 1-indexed choice (1..len(options)) or -1 on EOF.
func AskChoice(question string, options []string) int {
	r := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprintf(os.Stderr, "\n%s\n", question)
		for i, opt := range options {
			fmt.Fprintf(os.Stderr, "  %d) %s\n", i+1, opt)
		}
		fmt.Fprintf(os.Stderr, "Choice [1-%d]: ", len(options))
		line, err := r.ReadString('\n')
		if err != nil && err != io.EOF {
			return -1
		}
		ans := strings.TrimSpace(line)
		if ans == "" {
			return -1
		}
		var n int
		if _, err := fmt.Sscanf(ans, "%d", &n); err != nil {
			fmt.Fprintf(os.Stderr, "  (please enter a number 1-%d)\n", len(options))
			continue
		}
		if n < 1 || n > len(options) {
			fmt.Fprintf(os.Stderr, "  (out of range; please enter 1-%d)\n", len(options))
			continue
		}
		return n
	}
}

// ConfirmAction asks the user to type "yes" to confirm. Returns true only
// on exact "yes" (case-insensitive). Used for destructive operations like
// overwriting an existing config file.
func ConfirmAction(question string) bool {
	r := bufio.NewReader(os.Stdin)
	fmt.Fprintf(os.Stderr, "\n%s\nType 'yes' to confirm: ", question)
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return false
	}
	return strings.TrimSpace(strings.ToLower(line)) == "yes"
}
