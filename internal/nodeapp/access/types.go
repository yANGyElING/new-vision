// Package access contains the node-access (SIP/GB28181 control plane)
// JSON-RPC client and the Redis runtime projection. Its behavior is
// unchanged from the pre-refactor nodeapp package; only the package
// location moved.
package access

import (
	"context"
	"encoding/json"
	"time"
)

// AccessProfile is the profile pushed to node-access for one device.
type AccessProfile struct {
	DeviceAccessID  string `json:"device_access_id"`
	SIPUsername     string `json:"sip_username"`
	SIPRealm        string `json:"sip_realm"`
	DigestAlgorithm string `json:"digest_algorithm"`
	DigestHA1       string `json:"digest_ha1"`
	Enabled         bool   `json:"enabled"`
	Version         int64  `json:"version"`
}

type ProfileResult struct {
	Status  string `json:"status"`
	Version int64  `json:"version,omitempty"`
}

type RuntimeState struct {
	State         string    `json:"state"`
	Reason        string    `json:"reason,omitempty"`
	RemoteAddress string    `json:"remote_address,omitempty"`
	ExpiresAt     time.Time `json:"expires_at,omitempty"`
	LastSeen      time.Time `json:"last_seen,omitempty"`
	SessionEpoch  string    `json:"session_epoch,omitempty"`
	Stale         bool      `json:"stale"`
}

type RuntimeRegistration struct {
	DeviceAccessID string    `json:"device_access_id"`
	State          string    `json:"state"`
	Reason         string    `json:"reason,omitempty"`
	RemoteAddress  string    `json:"remote_address,omitempty"`
	ExpiresAt      time.Time `json:"expires_at,omitempty"`
	LastSeen       time.Time `json:"last_seen,omitempty"`
}

type RuntimeSnapshot struct {
	AccessInstanceID string                `json:"access_instance_id"`
	SessionEpoch     string                `json:"session_epoch"`
	SnapshotAt       time.Time             `json:"snapshot_at"`
	LatestSequence   int64                 `json:"latest_sequence"`
	Registrations    []RuntimeRegistration `json:"registrations"`
}

type AccessEventPayload struct {
	State         string    `json:"state"`
	Reason        string    `json:"reason"`
	RemoteAddress string    `json:"remote_address,omitempty"`
	ExpiresAt     time.Time `json:"expires_at,omitempty"`
	LastSeen      time.Time `json:"last_seen,omitempty"`
}

// CatalogChannel is one channel entry inside a catalog.result event.
// Empty Name or Status means "not reported this time"; the consumer keeps
// the stored value (design D43).
type CatalogChannel struct {
	Code   string `json:"code"`
	Name   string `json:"name,omitempty"`
	Status string `json:"status,omitempty"`
}

// CatalogProgressPayload is the body of a catalog.progress event: the
// aggregation state after one Catalog frame (design D50).
type CatalogProgressPayload struct {
	SN       int64 `json:"sn"`
	Received int   `json:"received"`
	Total    int   `json:"total"`
}

// CatalogResultPayload is the body of a catalog.result event. A failed sync
// (ok=false) carries the last received/total so the UI can show "received
// 26/32 then timed out"; Channels is empty in that case.
type CatalogResultPayload struct {
	OK       bool             `json:"ok"`
	Channels []CatalogChannel `json:"channels"`
	Error    string           `json:"error,omitempty"`
	Received int              `json:"received,omitempty"`
	Total    int              `json:"total,omitempty"`
}

type AccessEvent struct {
	EventID          string    `json:"event_id"`
	Sequence         int64     `json:"sequence"`
	AccessInstanceID string    `json:"access_instance_id"`
	SessionEpoch     string    `json:"session_epoch"`
	Type             string    `json:"type"`
	OccurredAt       time.Time `json:"occurred_at"`
	DeviceAccessID   string    `json:"device_access_id"`
	// Payload is kept raw: its shape depends on Type (registration_changed,
	// catalog.progress, catalog.result) and is decoded by the consumer.
	Payload json.RawMessage `json:"payload"`
}

type PollResult struct {
	AccessInstanceID string        `json:"access_instance_id"`
	SessionEpoch     string        `json:"session_epoch"`
	LatestSequence   int64         `json:"latest_sequence"`
	Events           []AccessEvent `json:"events"`
}

// RuntimeReader reads runtime state for device ids.
type RuntimeReader interface {
	Get(context.Context, string) (*RuntimeState, error)
	GetMany(context.Context, []string) (map[string]*RuntimeState, error)
}
