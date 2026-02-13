package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Entry struct {
	Time      time.Time     `json:"time"`
	Direction string        `json:"direction"` // "send" | "receive"
	Filename  string        `json:"filename"`
	Size      int64         `json:"size"`
	Duration  time.Duration `json:"duration"`
	Speed     float64       `json:"speed"` // bytes/sec
	Status    string        `json:"status"`
	Checksum  string        `json:"checksum"`
}

func historyFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".pulse")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "history.json"), nil
}

func LoadEntries() ([]Entry, error) {
	path, err := historyFile()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, err
	}

	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func SaveEntry(e Entry) error {
	entries, err := LoadEntries()
	if err != nil {
		return err
	}

	// Keep last 100 entries
	entries = append(entries, e)
	if len(entries) > 100 {
		entries = entries[len(entries)-100:]
	}

	path, err := historyFile()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

func PrintHistory() error {
	entries, err := LoadEntries()
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		fmt.Println("\n  No transfer history\n")
		return nil
	}

	fmt.Println("\n  📋 Transfer History\n")
	fmt.Println("  Time                | Dir  | File                    | Size    | Speed    | Status")
	fmt.Println("  " + string([]byte{'-'}) + string([]rune(make([]rune, 100, 100))[0:0]))

	for _, e := range entries {
		timeStr := e.Time.Format("2006-01-02 15:04:05")
		dirStr := e.Direction
		if e.Direction == "send" {
			dirStr = "↑"
		} else {
			dirStr = "↓"
		}

		sizeStr := formatBytes(e.Size)
		speedStr := formatBytes(int64(e.Speed)) + "/s"

		filename := e.Filename
		if len(filename) > 23 {
			filename = filename[:20] + "..."
		}

		fmt.Printf("  %-19s | %s  | %-23s | %-7s | %-8s | %s\n",
			timeStr, dirStr, filename, sizeStr, speedStr, e.Status)
	}
	fmt.Println()
	return nil
}

func formatBytes(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	} else if b < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
}
