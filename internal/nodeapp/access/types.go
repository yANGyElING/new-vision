// Package access contains the node-access (SIP/GB28181 control plane)
// JSON-RPC client and the Redis runtime projection. Its behavior is
// unchanged from the pre-refactor nodeapp package; only the package
// location moved.
package access

import (
	"context"
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

type AccessEvent struct {
	EventID          string             `json:"event_id"`
	Sequence         int64              `json:"sequence"`
	AccessInstanceID string             `json:"access_instance_id"`
	SessionEpoch     string             `json:"session_epoch"`
	Type             string             `json:"type"`
	OccurredAt       time.Time          `json:"occurred_at"`
	DeviceAccessID   string             `json:"device_access_id"`
	Payload          AccessEventPayload `json:"payload"`
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
