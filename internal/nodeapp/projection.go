package nodeapp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const nodeAppRedisPrefix = "nv:nodeapp:v1:"

type RedisProjection struct{ client *redis.Client }

func NewRedisProjection(client *redis.Client) *RedisProjection {
	return &RedisProjection{client: client}
}

func runtimeKey(deviceID string) string  { return nodeAppRedisPrefix + "device-runtime:" + deviceID }
func runtimeIndexKey() string            { return nodeAppRedisPrefix + "device-runtime-ids" }
func cursorKey(instance string) string   { return nodeAppRedisPrefix + "access-cursor:" + instance }
func snapshotKey(instance string) string { return nodeAppRedisPrefix + "access-snapshot:" + instance }

func (p *RedisProjection) Get(ctx context.Context, deviceID string) (*RuntimeState, error) {
	values, err := p.client.HGetAll(ctx, runtimeKey(deviceID)).Result()
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, nil
	}
	state := &RuntimeState{State: values["state"], Reason: values["reason"], RemoteAddress: values["remote_address"], SessionEpoch: values["session_epoch"]}
	state.Stale, _ = strconv.ParseBool(values["stale"])
	for key, target := range map[string]*time.Time{"expires_at": &state.ExpiresAt, "last_seen": &state.LastSeen} {
		if values[key] != "" {
			parsed, err := time.Parse(time.RFC3339Nano, values[key])
			if err != nil {
				return nil, fmt.Errorf("decode runtime %s: %w", key, err)
			}
			*target = parsed
		}
	}
	return state, nil
}

func (p *RedisProjection) Apply(ctx context.Context, deviceID string, state RuntimeState) error {
	pipe := p.client.TxPipeline()
	pipe.HSet(ctx, runtimeKey(deviceID), map[string]any{
		"state": state.State, "reason": state.Reason, "remote_address": state.RemoteAddress,
		"session_epoch": state.SessionEpoch, "stale": strconv.FormatBool(state.Stale),
		"expires_at": formatTime(state.ExpiresAt), "last_seen": formatTime(state.LastSeen),
	})
	pipe.SAdd(ctx, runtimeIndexKey(), deviceID)
	_, err := pipe.Exec(ctx)
	return err
}

func (p *RedisProjection) Replace(ctx context.Context, states map[string]RuntimeState, snapshot RuntimeSnapshot) error {
	ids, err := p.client.SMembers(ctx, runtimeIndexKey()).Result()
	if err != nil {
		return err
	}
	pipe := p.client.TxPipeline()
	keys := make([]string, 0, len(ids)+1)
	keys = append(keys, runtimeIndexKey())
	for _, id := range ids {
		keys = append(keys, runtimeKey(id))
	}
	pipe.Del(ctx, keys...)
	for id, state := range states {
		pipe.HSet(ctx, runtimeKey(id), map[string]any{
			"state": state.State, "reason": state.Reason, "remote_address": state.RemoteAddress,
			"session_epoch": state.SessionEpoch, "stale": strconv.FormatBool(state.Stale),
			"expires_at": formatTime(state.ExpiresAt), "last_seen": formatTime(state.LastSeen),
		})
		pipe.SAdd(ctx, runtimeIndexKey(), id)
	}
	pipe.Set(ctx, snapshotKey(snapshot.AccessInstanceID), mustJSON(snapshot), 0)
	_, err = pipe.Exec(ctx)
	return err
}

func (p *RedisProjection) Cursor(ctx context.Context, instance string) (string, int64, error) {
	values, err := p.client.HGetAll(ctx, cursorKey(instance)).Result()
	if err != nil {
		return "", 0, err
	}
	sequence, err := strconv.ParseInt(values["sequence"], 10, 64)
	if values["sequence"] == "" {
		sequence = 0
		err = nil
	}
	return values["epoch"], sequence, err
}

func (p *RedisProjection) SetCursor(ctx context.Context, instance, epoch string, sequence int64) error {
	return p.client.HSet(ctx, cursorKey(instance), map[string]any{"epoch": epoch, "sequence": sequence}).Err()
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
func mustJSON(value any) string { encoded, _ := json.Marshal(value); return string(encoded) }
