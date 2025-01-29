package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/storacha/indexing-service/pkg/types"
)

// DefaultExpire is the expire time we set on Redis when Set/SetExpiration are called with expire=true
const DefaultExpire = time.Hour

// Lifecycler is the subset of golang redis client functions we need to provide
// lifecycle management for a key.
type Lifecycler interface {
	Expire(ctx context.Context, key string, expiration time.Duration) *redis.BoolCmd
	Persist(ctx context.Context, key string) *redis.BoolCmd
}

// Client is a subset of functions from the golang redis client that we need to implement our cache
type Client interface {
	Lifecycler
	Get(context.Context, string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
}

// SetsClient is a subset of functions from the golang redis client that we need
// to implement a cache that stores multiple values for a key in a set.
type SetsClient interface {
	Lifecycler
	SAdd(ctx context.Context, key string, members ...interface{}) *redis.IntCmd
	SMembers(ctx context.Context, key string) *redis.StringSliceCmd
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

// SetsStore wraps the go redis client to implement our general purpose cache
// interface that works with sets, using the provided serialization/deserialization
// functions.
type SetsStore[Key, Value any] struct {
	fromRedis func(string) (Value, error)
	toRedis   func(Value) (string, error)
	keyString func(Key) string
	client    SetsClient
}

// NewSetsStore returns a new instance of a redis store with the provided
// serialization/deserialization functions.
func NewSetsStore[Key, Value any](
	fromRedis func(string) (Value, error),
	toRedis func(Value) (string, error),
	keyString func(Key) string,
	client SetsClient) *SetsStore[Key, Value] {
	return &SetsStore[Key, Value]{fromRedis, toRedis, keyString, client}
}

// Get returns all deserialized set values from redis.
func (rs *SetsStore[Key, Value]) Get(ctx context.Context, key Key) ([]Value, error) {
	data, err := rs.client.SMembers(ctx, rs.keyString(key)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, types.ErrKeyNotFound
		}
		return nil, fmt.Errorf("getting set members: %w", err)
	}
	var values []Value
	for _, d := range data {
		v, err := rs.fromRedis(d)
		if err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	return values, nil
}

// Add another value to the set of values for the given key.
func (rs *SetsStore[Key, Value]) Add(ctx context.Context, key Key, values ...Value) (uint64, error) {
	var data []any
	for _, v := range values {
		d, err := rs.toRedis(v)
		if err != nil {
			return 0, err
		}
		data = append(data, d)
	}
	n, err := rs.client.SAdd(ctx, rs.keyString(key), data...).Result()
	if err != nil {
		return 0, fmt.Errorf("adding set member: %w", err)
	}
	return uint64(n), nil
}

// SetExpirable changes the expiration property for a given key
func (rs *SetsStore[Key, Value]) SetExpirable(ctx context.Context, key Key, expires bool) error {
	if expires {
		err := rs.client.Expire(ctx, rs.keyString(key), DefaultExpire).Err()
		if err != nil {
			return fmt.Errorf("setting expire: %w", err)
		}
	} else {
		err := rs.client.Persist(ctx, rs.keyString(key)).Err()
		if err != nil {
			return fmt.Errorf("setting persist: %w", err)
		}
	}
	return nil
}
