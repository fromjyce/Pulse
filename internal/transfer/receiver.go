package transfer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gorilla/websocket"
	"github.com/fromjyce/pulse/internal/crypto"
)

type Receiver struct {
	relayURL string
	token    string
	key      []byte
	conn     *websocket.Conn
	debug    bool
}

func NewReceiver(relayURL, token string, key []byte) *Receiver {
	return &Receiver{relayURL: relayURL, token: token, key: key}
}

func NewReceiverWithDebug(relayURL, token string, key []byte, debug bool) *Receiver {
	return &Receiver{relayURL: relayURL, token: token, key: key, debug: debug}
}

func (r *Receiver) debugLog(msg string, args ...interface{}) {
	if r.debug {
		fmt.Printf("[DEBUG] "+msg+"\n", args...)
	}
}

func (r *Receiver) Connect() error {
	url := fmt.Sprintf("%s/ws/%s", r.relayURL, r.token)
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to relay: %w", err)
	}
	r.conn = conn

	readyMsg := NewReadyMessage()
	encryptedReady, err := crypto.EncryptChunk(EncodeMessage(readyMsg), r.key)
	if err != nil {
		return fmt.Errorf("failed to encrypt ready message: %w", err)
	}

	if err := r.conn.WriteMessage(websocket.BinaryMessage, encryptedReady); err != nil {
		return fmt.Errorf("failed to send ready message: %w", err)
	}

	r.debugLog("Receiver connected and ready")
	return nil
}

func (r *Receiver) ReceiveFile(ctx context.Context, destDir string, progressFn func(received, total int64)) (string, Stats, error) {
	startTime := time.Now()
	stats := Stats{}

	var metadata Metadata
	var file *os.File
	var bytesReceived int64
	var destPath string
	var fileContent []byte

	defer func() {
		if file != nil {
			file.Close()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			// Context cancelled, clean up and return
			if file != nil {
				os.Remove(destPath)
			}
			return "", stats, fmt.Errorf("transfer cancelled by receiver")
		default:
		}

		r.conn.SetReadDeadline(time.Now().Add(5 * time.Minute))

		_, encryptedData, err := r.conn.ReadMessage()
		if err != nil {
			if file != nil {
				os.Remove(destPath)
			}
			return "", stats, fmt.Errorf("failed to read message: %w", err)
		}

		decrypted, err := crypto.DecryptChunk(encryptedData, r.key)
		if err != nil {
			if file != nil {
				os.Remove(destPath)
			}
			return "", stats, fmt.Errorf("failed to decrypt message: %w", err)
		}

		msg, err := DecodeMessage(decrypted)
		if err != nil {
			if file != nil {
				os.Remove(destPath)
			}
			return "", stats, fmt.Errorf("failed to decode message: %w", err)
		}

		switch msg.Type {
		case MsgTypeMetadata:
			metadata, err = ParseMetadata(msg.Payload)
			if err != nil {
				return "", stats, fmt.Errorf("failed to parse metadata: %w", err)
			}
			r.debugLog("Received metadata: %s (%d bytes, checksum: %s)", metadata.Filename, metadata.Size, metadata.Checksum)
			destPath = filepath.Join(destDir, metadata.Filename)
			file, err = os.Create(destPath)
			if err != nil {
				return "", stats, fmt.Errorf("failed to create file: %w", err)
			}
			fileContent = make([]byte, 0, metadata.Size)

		case MsgTypeChunk:
			if file == nil {
				return "", stats, fmt.Errorf("received chunk before metadata")
			}
			n, err := file.Write(msg.Payload)
			if err != nil {
				os.Remove(destPath)
				return "", stats, fmt.Errorf("failed to write chunk: %w", err)
			}
			fileContent = append(fileContent, msg.Payload...)
			bytesReceived += int64(n)
			if progressFn != nil {
				progressFn(bytesReceived, metadata.Size)
			}

		case MsgTypeComplete:
			// Verify checksum
			if metadata.Checksum != "" {
				r.debugLog("Verifying checksum...")
				computedChecksum := crypto.ComputeChecksum(fileContent)
				if computedChecksum != metadata.Checksum {
					os.Remove(destPath)
					return "", stats, fmt.Errorf("checksum mismatch: expected %s, got %s", metadata.Checksum, computedChecksum)
				}
				r.debugLog("Checksum verified ✓")
			}

			duration := time.Since(startTime)
			speed := float64(bytesReceived) / duration.Seconds()

			stats.Duration = duration
			stats.BytesSent = bytesReceived
			stats.Speed = speed

			r.debugLog("Transfer complete: %d bytes in %v (%.0f bytes/sec)", bytesReceived, duration, speed)
			return destPath, stats, nil

		case MsgTypeCancel:
			os.Remove(destPath)
			return "", stats, fmt.Errorf("sender cancelled transfer: %s", string(msg.Payload))

		case MsgTypeError:
			os.Remove(destPath)
			return "", stats, fmt.Errorf("sender error: %s", string(msg.Payload))
		}
	}
}

func (r *Receiver) Close() error {
	if r.conn != nil {
		return r.conn.Close()
	}
	return nil
}
