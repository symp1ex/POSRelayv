package console

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"posrelayd-viewer/internal/logger"
)

func ReadPassword(prompt string) (string, error) {
	logger.Posrelayv.Debug("[console] Reading password from stdin")
	fmt.Print(prompt)

	if term.IsTerminal(int(os.Stdin.Fd())) {
		logger.Posrelayv.Debug("[console] Stdin is terminal, using hidden password input")

		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			logger.Posrelayv.Warnf("[console] Failed to read password from terminal: %v", err)
			return "", err
		}
		return string(b), nil
	}

	logger.Posrelayv.Debug("[console] Stdin is not terminal, using line input fallback")

	// fallback (не-TTY)
	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil {
		logger.Posrelayv.Warnf("[console] Failed to read password from stdin fallback: %v", err)
		return "", err
	}
	return strings.TrimRight(text, "\r\n"), nil
}

func DrainStdin(reader *bufio.Reader) {
	drained := 0

	for reader.Buffered() > 0 {
		_, _ = reader.ReadString('\n')
		drained++
	}

	if drained > 0 {
		logger.Posrelayv.Debugf("[console] Drained buffered stdin lines: count=%d", drained)
	}
}
