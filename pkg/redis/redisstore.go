package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/storacha-network/indexing-service/pkg/types"
)

// DefaultExpire is the expire time we set on Redis when Set/SetExpiration are called with expire=true
const DefaultExpire = time.Hour

// Client is a subset of functions from the golang redis client that we need to implement our cache
type Client interface {
	Get(context.Context, string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	Expire(ctx context.Context, key string, expiration time.Duration) *redis.BoolCmd
	Persist(ctx context.Context, key string) *redis.BoolCmd
}

// Store wraps the go redis client to implement our general purpose cache interface,
// using the providedserialization/deserialization functions
type Store[Key, Value any] struct {
	fromRedis func(string) (Value, error)
	toRedis   func(Value) (string, error)
	keyString func(Key) string
	client    Client
}

var (
	_ Client                = (*redis.Client)(nil)
	_ types.Cache[any, any] = (*Store[any, any])(nil)
)

// NewStore returns a new instance of a redis store with the provided serialization/deserialization functions
func NewStore[Key, Value any](
	fromRedis func(string) (Value, error),
	toRedis func(Value) (string, error),
	keyString func(Key) string,
	client Client) *Store[Key, Value] {
	return &Store[Key, Value]{fromRedis, toRedis, keyString, client}
}

// Get returns deserialized values from redis
func (rs *Store[Key, Value]) Get(ctx context.Context, key Key) (Value, error) {
	data, err := rs.client.Get(ctx, rs.keyString(key)).Result()

	if err != nil {
		var v Value
		if err == redis.Nil {
			return v, types.ErrKeyNotFound
		}
		return v, fmt.Errorf("error accessing redis: %w", err)
	}
	return rs.fromRedis(data)
}

// Set saves a serialized value to redis
func (rs *Store[Key, Value]) Set(ctx context.Context, key Key, value Value, expires bool) error {
	data, err := rs.toRedis(value)
	if err != nil {
		return err
	}
	duration := time.Duration(0)
	if expires {
		duration = DefaultExpire
	}
	err = rs.client.Set(ctx, rs.keyString(key), data, duration).Err()
	if err != nil {
		return fmt.Errorf("error accessing redis: %w", err)
	}
	return nil
}

// SetExpirable changes the expiration property for a given key
func (rs *Store[Key, Value]) SetExpirable(ctx context.Context, key Key, expires bool) error {
	var err error
	if expires {
		err = rs.client.Expire(ctx, rs.keyString(key), DefaultExpire).Err()
	} else {
		err = rs.client.Persist(ctx, rs.keyString(key)).Err()
	}
	if err != nil {
		return fmt.Errorf("error accessing redis: %w", err)
	}
	return nil
}
