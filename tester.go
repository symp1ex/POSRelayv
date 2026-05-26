package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

type ClientData struct {
	ID       string
	Password string
}

func main() {

	clients, err := loadCSV("clients.csv")
	if err != nil {
		fmt.Println("CSV error:", err)
		return
	}

	logFile, err := os.Create("results.txt")
	if err != nil {
		fmt.Println("Log file error:", err)
		return
	}
	defer logFile.Close()

	for i, c := range clients {

		fmt.Printf("[%d/%d] %s\n", i+1, len(clients), c.ID)

		// Windows:
		cmd := exec.Command("F:\\Program Files\\POSRelayv\\POSRelayv.exe")

		// Linux/macOS:
		// cmd := exec.Command("./client")

		stdin, err := cmd.StdinPipe()
		if err != nil {
			fmt.Println(err)
			continue
		}

		stdout, _ := cmd.StdoutPipe()
		stderr, _ := cmd.StderrPipe()

		err = cmd.Start()
		if err != nil {
			fmt.Println("Start error:", err)
			continue
		}

		// отправляем ID
		_, _ = stdin.Write([]byte(c.ID + "\n"))

		time.Sleep(500 * time.Millisecond)

		// отправляем пароль
		_, _ = stdin.Write([]byte(c.Password + "\n"))

		time.Sleep(5 * time.Second)

		_, _ = stdin.Write([]byte("time /t\n"))

		// ждём ответа клиента
		time.Sleep(5 * time.Second)

		// убиваем процесс
		_ = cmd.Process.Kill()

		outBytes, _ := io.ReadAll(stdout)
		errBytes, _ := io.ReadAll(stderr)

		logText := fmt.Sprintf(
			"\n====================\nID: %s\nPASS: %s\nSTDOUT:\n%s\nSTDERR:\n%s\n",
			c.ID,
			c.Password,
			string(outBytes),
			string(errBytes),
		)

		_, _ = logFile.WriteString(logText)

		fmt.Println("DONE")
	}
}

func loadCSV(path string) ([]ClientData, error) {

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(bufio.NewReader(file))
	reader.Comma = ';'

	rows, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	var result []ClientData

	for i, row := range rows {

		if i == 0 {
			continue
		}

		if len(row) < 2 {
			continue
		}

		result = append(result, ClientData{
			ID:       strings.TrimSpace(row[0]),
			Password: strings.TrimSpace(row[1]),
		})
	}

	return result, nil
}
