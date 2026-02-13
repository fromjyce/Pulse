package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fromjyce/pulse/internal/crypto"
	"github.com/fromjyce/pulse/internal/history"
	"github.com/fromjyce/pulse/internal/notify"
	"github.com/fromjyce/pulse/internal/qr"
	"github.com/fromjyce/pulse/internal/transfer"
)

const defaultRelay = "wss://pulse.relay.app"

func main() {
	// Global flags
	relay := flag.String("relay", defaultRelay, "Relay server URL")
	debug := flag.Bool("debug", false, "Enable debug logging")
	chunkSize := flag.Int("chunk-size", 65536, "Chunk size in bytes (default 64KB)")
	timeout := flag.Duration("timeout", 5*time.Minute, "Transfer timeout (default 5m)")
	retries := flag.Int("retries", 3, "Number of connection retries (default 3)")
	notifyFlag := flag.Bool("notify", false, "Send desktop notification on completion")

	flag.Parse()
	args := flag.Args()

	if len(args) < 1 {
		printUsage()
		os.Exit(1)
	}

	var err error
	switch args[0] {
	case "send":
		if len(args) < 2 {
			fmt.Println("Usage: pulse send <file> [file2 file3 ...]")
			os.Exit(1)
		}
		err = cmdSend(*relay, args[1:], *debug, *chunkSize, *timeout, *retries, *notifyFlag)
	case "receive":
		dir := "."
		if len(args) >= 2 {
			dir = args[1]
		}
		err = cmdReceive(*relay, dir, *debug, *timeout, *notifyFlag)
	case "history":
		err = cmdHistory()
	default:
		fmt.Printf("Unknown command: %s\n", args[0])
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Printf("\n  ✗ Error: %v\n\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`
  Pulse - Secure file transfer between terminal and phone

  Usage:
    pulse send <file> [file2 file3 ...]    Send one or more files
    pulse receive [dir]                     Receive files
    pulse history                            Show transfer history

  Flags:
    --relay <url>       Relay server URL (default: wss://pulse.relay.app)
    --debug             Enable debug logging
    --chunk-size <n>    Chunk size in bytes (default: 65536)
    --timeout <d>       Transfer timeout (default: 5m)
    --retries <n>       Connection retries (default: 3)
    --notify            Send desktop notification on completion

  Examples:
    pulse send document.pdf
    pulse send file1.txt file2.txt file3.txt
    pulse receive ~/Downloads
    pulse --debug send config.yaml
`)
}

func cmdSend(relay string, filePaths []string, debug bool, chunkSize int, timeout time.Duration, retries int, notifyFlag bool) error {
	// Validate files exist
	for _, filePath := range filePaths {
		if _, err := os.Stat(filePath); err != nil {
			return fmt.Errorf("file not found: %s", filePath)
		}
	}

	token := genToken()
	key, _ := crypto.GenerateKey()

	httpRelay := strings.Replace(strings.Replace(relay, "wss://", "https://", 1), "ws://", "http://", 1)
	url := fmt.Sprintf("%s/d/%s#%s", httpRelay, token, crypto.KeyToBase64(key))

	fmt.Println("\n  🚀 Pulse - Send\n")
	if len(filePaths) == 1 {
		stat, _ := os.Stat(filePaths[0])
		fmt.Printf("  📄 File: %s (%s)\n\n", stat.Name(), fmtBytes(stat.Size()))
	} else {
		totalSize := int64(0)
		for _, fp := range filePaths {
			if stat, err := os.Stat(fp); err == nil {
				totalSize += stat.Size()
			}
		}
		fmt.Printf("  📦 Batch: %d files (%s total)\n\n", len(filePaths), fmtBytes(totalSize))
	}

	if err := qr.GenerateTerminal(url); err != nil {
		return err
	}

	fmt.Printf("\n  📲 %s\n\n  🔒 E2E Encrypted\n  ⏳ Waiting for receiver...\n\n", url)

	cfg := transfer.Config{
		ChunkSize: chunkSize,
		Timeout:   timeout,
		Retries:   retries,
		Debug:     debug,
	}

	sender := transfer.NewSender(relay, token, key, cfg)
	if err := sender.Connect(); err != nil {
		return err
	}
	defer sender.Close()

	if err := sender.WaitForReceiver(timeout); err != nil {
		return err
	}
	fmt.Println("  ✓ Connected!\n")

	// Setup signal handling for cancellation
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n  ⚠ Cancelling transfer...")
		cancel()
	}()

	startTime := time.Now()
	totalSize := int64(0)

	for i, filePath := range filePaths {
		if debug {
			fmt.Printf("  [DEBUG] Sending file %d/%d: %s\n", i+1, len(filePaths), filePath)
		}

		progressFn := makeProgressFn(filePaths[i])
		stats, err := sender.SendFile(ctx, filePath, progressFn)
		if err != nil {
			return err
		}

		stat, _ := os.Stat(filePath)
		totalSize += stat.Size()

		// Save to history
		histEntry := history.Entry{
			Time:      time.Now(),
			Direction: "send",
			Filename:  stat.Name(),
			Size:      stat.Size(),
			Duration:  stats.Duration,
			Speed:     stats.Speed,
			Status:    "ok",
		}
		history.SaveEntry(histEntry)
	}

	totalDuration := time.Since(startTime)
	avgSpeed := float64(totalSize) / totalDuration.Seconds()

	fmt.Printf("\n  ✓ Done! (%s in %v @ %.0f KB/s)\n", fmtBytes(totalSize), fmtDuration(totalDuration), avgSpeed/1024)
	fmt.Println("  ✓ Checksum verified\n")

	if notifyFlag {
		notify.Notify("Pulse", fmt.Sprintf("✓ Sent %d file(s) successfully", len(filePaths)))
	}

	return nil
}

func cmdReceive(relay, destDir string, debug bool, timeout time.Duration, notifyFlag bool) error {
	// Create destination directory if it doesn't exist
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	token := genToken()
	key, _ := crypto.GenerateKey()

	httpRelay := strings.Replace(strings.Replace(relay, "wss://", "https://", 1), "ws://", "http://", 1)
	url := fmt.Sprintf("%s/u/%s#%s", httpRelay, token, crypto.KeyToBase64(key))

	fmt.Println("\n  🚀 Pulse - Receive\n")
	fmt.Printf("  📍 Destination: %s\n\n", destDir)

	if err := qr.GenerateTerminal(url); err != nil {
		return err
	}

	fmt.Printf("\n  📲 %s\n\n  🔒 E2E Encrypted\n  ⏳ Waiting for sender...\n\n", url)

	receiver := transfer.NewReceiverWithDebug(relay, token, key, debug)
	if err := receiver.Connect(); err != nil {
		return err
	}
	defer receiver.Close()

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n  ⚠ Cancelling transfer...")
		cancel()
	}()

	progressFn := func(received, total int64) {
		pct := float64(received) / float64(total) * 100
		speed := float64(received) / time.Since(time.Now().Add(-time.Second)).Seconds()
		if speed == 0 {
			speed = 1 // Avoid division by zero
		}
		fmt.Printf("\r  [%-40s] %.0f%% | %.1f MB/s",
			strings.Repeat("█", int(pct/2.5))+strings.Repeat("░", 40-int(pct/2.5)),
			pct, speed/(1024*1024))
	}

	savedPath, stats, err := receiver.ReceiveFile(ctx, destDir, progressFn)
	if err != nil {
		return err
	}

	// Save to history
	fi, _ := os.Stat(savedPath)
	histEntry := history.Entry{
		Time:      time.Now(),
		Direction: "receive",
		Filename:  fi.Name(),
		Size:      fi.Size(),
		Duration:  stats.Duration,
		Speed:     stats.Speed,
		Status:    "ok",
	}
	history.SaveEntry(histEntry)

	fmt.Printf("\n  ✓ Saved: %s\n", savedPath)
	fmt.Printf("  ✓ Done! (%s in %v @ %.0f KB/s)\n", fmtBytes(stats.BytesSent), fmtDuration(stats.Duration), stats.Speed/1024)
	fmt.Println("  ✓ Checksum verified\n")

	if notifyFlag {
		notify.Notify("Pulse", fmt.Sprintf("✓ Received %s successfully", fi.Name()))
	}

	return nil
}

func cmdHistory() error {
	return history.PrintHistory()
}

func genToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func fmtBytes(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	} else if b < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	} else if b < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
	}
	return fmt.Sprintf("%.1f GB", float64(b)/(1024*1024*1024))
}

func fmtDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	} else if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%.1fm", d.Minutes())
}

func makeProgressFn(filePath string) func(sent, total int64) {
	return func(sent, total int64) {
		pct := float64(sent) / float64(total) * 100
		fmt.Printf("\r  [%-40s] %.0f%%",
			strings.Repeat("█", int(pct/2.5))+strings.Repeat("░", 40-int(pct/2.5)),
			pct)
	}
}
