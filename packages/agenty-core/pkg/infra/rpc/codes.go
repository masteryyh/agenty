package rpc

// Standard JSON-RPC 2.0 error codes (reserved range -32768 to -32000).
const (
	ErrCodeParseError     = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternalError  = -32603
)

// Server-defined error codes
const (
	ErrCodeNotFound             = -32001
	ErrCodeAlreadyExists        = -32002
	ErrCodeMessageTooLarge      = -32003
	ErrCodeChunkPayloadTooLarge = -32004
)
