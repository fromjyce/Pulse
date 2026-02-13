package transfer

import (
	"encoding/binary"
	"encoding/json"
	"errors"
)

type MessageType byte

const (
	MsgTypeMetadata MessageType = 0x01
	MsgTypeReady    MessageType = 0x02
	MsgTypeComplete MessageType = 0x03
	MsgTypeError    MessageType = 0x04
	MsgTypeChunk    MessageType = 0x10
	MsgTypeProgress MessageType = 0x11
	MsgTypeCancel   MessageType = 0x05
	MsgTypeChecksum MessageType = 0x06
)

type Metadata struct {
	Filename   string `json:"filename"`
	Size       int64  `json:"size"`
	Chunks     int    `json:"chunks"`
	Checksum   string `json:"checksum"`    // SHA256 hex
	MimeType   string `json:"mime_type"`   // detected MIME type
	BatchIndex int    `json:"batch_index"` // 0-based index in batch
	BatchTotal int    `json:"batch_total"` // total files in batch
}

type Progress struct {
	ChunkIndex  int   `json:"chunk_index"`
	TotalChunks int   `json:"total_chunks"`
	BytesSent   int64 `json:"bytes_sent"`
	TotalBytes  int64 `json:"total_bytes"`
}

type Message struct {
	Type    MessageType
	Payload []byte
}

func EncodeMessage(msg Message) []byte {
	result := make([]byte, 5+len(msg.Payload))
	result[0] = byte(msg.Type)
	binary.BigEndian.PutUint32(result[1:5], uint32(len(msg.Payload)))
	copy(result[5:], msg.Payload)
	return result
}

func DecodeMessage(data []byte) (Message, error) {
	if len(data) < 5 {
		return Message{}, errors.New("message too short")
	}
	msgType := MessageType(data[0])
	payloadLen := binary.BigEndian.Uint32(data[1:5])
	if len(data) < int(5+payloadLen) {
		return Message{}, errors.New("incomplete message")
	}
	return Message{
		Type:    msgType,
		Payload: data[5 : 5+payloadLen],
	}, nil
}

func NewMetadataMessage(meta Metadata) (Message, error) {
	payload, err := json.Marshal(meta)
	if err != nil {
		return Message{}, err
	}
	return Message{Type: MsgTypeMetadata, Payload: payload}, nil
}

func NewChunkMessage(data []byte) Message {
	return Message{Type: MsgTypeChunk, Payload: data}
}

func NewReadyMessage() Message {
	return Message{Type: MsgTypeReady, Payload: nil}
}

func NewCompleteMessage() Message {
	return Message{Type: MsgTypeComplete, Payload: nil}
}

func NewErrorMessage(errStr string) Message {
	return Message{Type: MsgTypeError, Payload: []byte(errStr)}
}

func NewCancelMessage(reason string) Message {
	return Message{Type: MsgTypeCancel, Payload: []byte(reason)}
}

func NewChecksumMessage(checksum string) Message {
	return Message{Type: MsgTypeChecksum, Payload: []byte(checksum)}
}

func ParseMetadata(payload []byte) (Metadata, error) {
	var meta Metadata
	err := json.Unmarshal(payload, &meta)
	return meta, err
}
