package access

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const maxRPCResponseBytes = 16 << 20

// AccessAPI is the node-access JSON-RPC surface used by node-app.
type AccessAPI interface {
	ApplyDeviceProfile(context.Context, AccessProfile) (ProfileResult, error)
	RemoveDeviceProfile(context.Context, string, int64) (ProfileResult, error)
	ReplaceDeviceProfiles(context.Context, []AccessProfile) error
	GetRuntimeSnapshot(context.Context) (RuntimeSnapshot, error)
	PollEvents(context.Context, int64, int) (PollResult, error)
	AckEvents(context.Context, int64) error
}

type RPCError struct {
	Code     int
	Message  string
	DataCode string
}

func (e *RPCError) Error() string {
	if e.DataCode != "" {
		return fmt.Sprintf("access RPC error %s", e.DataCode)
	}
	return fmt.Sprintf("access RPC error %d: %s", e.Code, e.Message)
}

func IsRPCErrorCode(err error, code string) bool {
	var rpcErr *RPCError
	return errors.As(err, &rpcErr) && rpcErr.DataCode == code
}

type AccessClient struct {
	url    string
	client *http.Client
}

func NewAccessClient(url string, timeout time.Duration) *AccessClient {
	return &AccessClient{url: url, client: &http.Client{Timeout: timeout}}
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      string `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Code string `json:"code"`
		} `json:"data"`
	} `json:"error"`
}

func (c *AccessClient) call(ctx context.Context, method string, params any, result any) error {
	id, err := newUUID()
	if err != nil {
		return fmt.Errorf("generate request id: %w", err)
	}
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params})
	if err != nil {
		return fmt.Errorf("encode access request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create access request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("call access: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("call access: unexpected HTTP status %d", resp.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxRPCResponseBytes+1))
	var envelope rpcResponse
	if err := decoder.Decode(&envelope); err != nil {
		return fmt.Errorf("decode access response: %w", err)
	}
	if envelope.JSONRPC != "2.0" || envelope.ID != id {
		return errors.New("decode access response: mismatched JSON-RPC identity")
	}
	if envelope.Error != nil {
		return &RPCError{Code: envelope.Error.Code, Message: envelope.Error.Message, DataCode: envelope.Error.Data.Code}
	}
	if len(envelope.Result) == 0 {
		return errors.New("decode access response: missing result")
	}
	if result == nil {
		return nil
	}
	if err := json.Unmarshal(envelope.Result, result); err != nil {
		return fmt.Errorf("decode access result: %w", err)
	}
	return nil
}

func (c *AccessClient) ApplyDeviceProfile(ctx context.Context, profile AccessProfile) (ProfileResult, error) {
	requestID, err := newUUID()
	if err != nil {
		return ProfileResult{}, err
	}
	var result ProfileResult
	err = c.call(ctx, "access.v1.applyDeviceProfile", struct {
		RequestID string        `json:"request_id"`
		Profile   AccessProfile `json:"profile"`
	}{requestID, profile}, &result)
	return result, err
}

func (c *AccessClient) RemoveDeviceProfile(ctx context.Context, accessID string, version int64) (ProfileResult, error) {
	requestID, err := newUUID()
	if err != nil {
		return ProfileResult{}, err
	}
	var result ProfileResult
	err = c.call(ctx, "access.v1.removeDeviceProfile", struct {
		RequestID      string `json:"request_id"`
		DeviceAccessID string `json:"device_access_id"`
		Version        int64  `json:"version"`
	}{requestID, accessID, version}, &result)
	return result, err
}

func (c *AccessClient) ReplaceDeviceProfiles(ctx context.Context, profiles []AccessProfile) error {
	requestID, err := newUUID()
	if err != nil {
		return err
	}
	generationID, err := newUUID()
	if err != nil {
		return err
	}
	var result struct {
		Status string `json:"status"`
	}
	return c.call(ctx, "access.v1.replaceDeviceProfiles", struct {
		RequestID    string          `json:"request_id"`
		GenerationID string          `json:"generation_id"`
		Profiles     []AccessProfile `json:"profiles"`
	}{requestID, generationID, profiles}, &result)
}

func (c *AccessClient) GetRuntimeSnapshot(ctx context.Context) (RuntimeSnapshot, error) {
	requestID, err := newUUID()
	if err != nil {
		return RuntimeSnapshot{}, err
	}
	var result RuntimeSnapshot
	err = c.call(ctx, "access.v1.getRuntimeSnapshot", struct {
		RequestID string `json:"request_id"`
	}{requestID}, &result)
	return result, err
}

func (c *AccessClient) PollEvents(ctx context.Context, after int64, limit int) (PollResult, error) {
	requestID, err := newUUID()
	if err != nil {
		return PollResult{}, err
	}
	var result PollResult
	err = c.call(ctx, "access.v1.pollEvents", struct {
		RequestID     string `json:"request_id"`
		AfterSequence int64  `json:"after_sequence"`
		Limit         int    `json:"limit"`
	}{requestID, after, limit}, &result)
	return result, err
}

func (c *AccessClient) AckEvents(ctx context.Context, through int64) error {
	requestID, err := newUUID()
	if err != nil {
		return err
	}
	var result struct {
		Status string `json:"status"`
	}
	return c.call(ctx, "access.v1.ackEvents", struct {
		RequestID       string `json:"request_id"`
		ThroughSequence int64  `json:"through_sequence"`
	}{requestID, through}, &result)
}

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	text := make([]byte, 36)
	hex.Encode(text[0:8], b[0:4])
	text[8] = '-'
	hex.Encode(text[9:13], b[4:6])
	text[13] = '-'
	hex.Encode(text[14:18], b[6:8])
	text[18] = '-'
	hex.Encode(text[19:23], b[8:10])
	text[23] = '-'
	hex.Encode(text[24:36], b[10:16])
	return string(text), nil
}
