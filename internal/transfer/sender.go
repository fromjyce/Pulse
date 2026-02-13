package transfer

import (
	"context"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"time"

	"github.com/gorilla/websocket"
	"github.com/fromjyce/pulse/internal/crypto"
)

const DefaultChunkSize = 64 * 1024

type Config struct {
	ChunkSize int           // default 64KB
	Timeout   time.Duration // default 5 min
	Retries   int           // default 3
	Debug     bool
}

type Stats struct {
	Duration  time.Duration
	BytesSent int64
	Speed     float64 // bytes/sec
}

type Sender struct {
	relayURL string
	token    string
	key      []byte
	conn     *websocket.Conn
	config   Config
}

func NewSender(relayURL, token string, key []byte, cfg Config) *Sender {
	if cfg.ChunkSize == 0 {
		cfg.ChunkSize = DefaultChunkSize
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Minute
	}
	if cfg.Retries == 0 {
		cfg.Retries = 3
	}
	return &Sender{relayURL: relayURL, token: token, key: key, config: cfg}
}

func (s *Sender) debug(msg string, args ...interface{}) {
	if s.config.Debug {
		fmt.Printf("[DEBUG] "+msg+"\n", args...)
	}
}

func (s *Sender) Connect() error {
	var lastErr error
	for attempt := 0; attempt < s.config.Retries; attempt++ {
		s.debug("Connect attempt %d/%d", attempt+1, s.config.Retries)
		url := fmt.Sprintf("%s/ws/%s", s.relayURL, s.token)
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err == nil {
			s.conn = conn
			s.debug("Connected successfully")
			return nil
		}
		lastErr = err
		if attempt < s.config.Retries-1 {
			backoff := time.Duration((attempt+1)*2) * time.Second
			s.debug("Connection failed, retrying in %v: %v", backoff, err)
			time.Sleep(backoff)
		}
	}
	return fmt.Errorf("failed to connect to relay after %d attempts: %w", s.config.Retries, lastErr)
}

func (s *Sender) WaitForReceiver(timeout time.Duration) error {
	s.conn.SetReadDeadline(time.Now().Add(timeout))
	defer s.conn.SetReadDeadline(time.Time{})

	_, message, err := s.conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("timeout waiting for receiver: %w", err)
	}

	decrypted, err := crypto.DecryptChunk(message, s.key)
	if err != nil {
		return fmt.Errorf("failed to decrypt ready message: %w", err)
	}

	msg, err := DecodeMessage(decrypted)
	if err != nil {
		return err
	}
	if msg.Type != MsgTypeReady {
		return fmt.Errorf("unexpected message type: %d", msg.Type)
	}
	s.debug("Receiver ready")
	return nil
}

func (s *Sender) SendFile(ctx context.Context, filePath string, progressFn func(sent, total int64)) (Stats, error) {
	startTime := time.Now()
	stats := Stats{}

	file, err := os.Open(filePath)
	if err != nil {
		return stats, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return stats, fmt.Errorf("failed to stat file: %w", err)
	}

	// Compute checksum
	s.debug("Computing checksum for %s", stat.Name())
	checksumData, err := io.ReadAll(file)
	if err != nil {
		return stats, fmt.Errorf("failed to read file for checksum: %w", err)
	}
	checksum := crypto.ComputeChecksum(checksumData)
	s.debug("Checksum: %s", checksum)

	// Reset file pointer
	if _, err := file.Seek(0, 0); err != nil {
		return stats, fmt.Errorf("failed to seek file: %w", err)
	}

	filename := filepath.Base(filePath)
	fileSize := stat.Size()
	totalChunks := int((fileSize + int64(s.config.ChunkSize) - 1) / int64(s.config.ChunkSize))

	// Detect MIME type
	mimeType := mime.TypeByExtension(filepath.Ext(filePath))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	meta := Metadata{
		Filename:   filename,
		Size:       fileSize,
		Chunks:     totalChunks,
		Checksum:   checksum,
		MimeType:   mimeType,
		BatchIndex: 0,
		BatchTotal: 1,
	}

	metaMsg, err := NewMetadataMessage(meta)
	if err != nil {
		return stats, err
	}

	encryptedMeta, err := crypto.EncryptChunk(EncodeMessage(metaMsg), s.key)
	if err != nil {
		return stats, fmt.Errorf("failed to encrypt metadata: %w", err)
	}

	if err := s.conn.WriteMessage(websocket.BinaryMessage, encryptedMeta); err != nil {
		return stats, fmt.Errorf("failed to send metadata: %w", err)
	}

	buf := make([]byte, s.config.ChunkSize)
	var bytesSent int64

	for {
		select {
		case <-ctx.Done():
			// Send cancel message
			cancelMsg := NewCancelMessage("cancelled by sender")
			if encMsg, err := crypto.EncryptChunk(EncodeMessage(cancelMsg), s.key); err == nil {
				s.conn.WriteMessage(websocket.BinaryMessage, encMsg)
			}
			return stats, ctx.Err()
		default:
		}

		n, err := file.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			return stats, fmt.Errorf("failed to read file: %w", err)
		}

		chunkMsg := NewChunkMessage(buf[:n])
		encryptedChunk, err := crypto.EncryptChunk(EncodeMessage(chunkMsg), s.key)
		if err != nil {
			return stats, fmt.Errorf("failed to encrypt chunk: %w", err)
		}

		if err := s.conn.WriteMessage(websocket.BinaryMessage, encryptedChunk); err != nil {
			return stats, fmt.Errorf("failed to send chunk: %w", err)
		}

		bytesSent += int64(n)
		if progressFn != nil {
			progressFn(bytesSent, fileSize)
		}
	}

	completeMsg := NewCompleteMessage()
	encryptedComplete, err := crypto.EncryptChunk(EncodeMessage(completeMsg), s.key)
	if err != nil {
		return stats, fmt.Errorf("failed to encrypt complete message: %w", err)
	}

	if err := s.conn.WriteMessage(websocket.BinaryMessage, encryptedComplete); err != nil {
		return stats, fmt.Errorf("failed to send complete message: %w", err)
	}

	duration := time.Since(startTime)
	speed := float64(bytesSent) / duration.Seconds()

	stats.Duration = duration
	stats.BytesSent = bytesSent
	stats.Speed = speed

	s.debug("Transfer complete: %d bytes in %v (%.0f bytes/sec)", bytesSent, duration, speed)
	return stats, nil
}

func (s *Sender) Close() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}
