package console

import (
	"bufio"
	"fmt"
	"golang.org/x/term"
	"os"
	"strings"
)

func ReadPassword(prompt string) (string, error) {
	fmt.Print(prompt)

	if term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", err
		}
		return string(b), nil
	}

	// fallback (не-TTY)
	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(text, "\r\n"), nil
}

func DrainStdin(reader *bufio.Reader) {
	for reader.Buffered() > 0 {
		_, _ = reader.ReadString('\n')
	}
}
