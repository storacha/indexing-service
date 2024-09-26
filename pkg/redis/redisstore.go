package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/storacha-network/indexing-service/pkg/types"
)

const DefaultExpire = time.Hour

type RedisClient interface {
	Get(context.Context, string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	Expire(ctx context.Context, key string, expiration time.Duration) *redis.BoolCmd
	Persist(ctx context.Context, key string) *redis.BoolCmd
}

type RedisStore[Key, Value any] struct {
	fromRedis func(string) (Value, error)
	toRedis   func(Value) (string, error)
	keyString func(Key) string
	client    RedisClient
}

var (
	_ RedisClient           = (*redis.Client)(nil)
	_ types.Cache[any, any] = (*RedisStore[any, any])(nil)
)

func NewRedisStore[Key, Value any](
	fromRedis func(string) (Value, error),
	toRedis func(Value) (string, error),
	keyString func(Key) string,
	client RedisClient) *RedisStore[Key, Value] {
	return &RedisStore[Key, Value]{fromRedis, toRedis, keyString, client}
}

// Get returns all provider results from redis
func (rs *RedisStore[Key, Value]) Get(ctx context.Context, key Key) (Value, error) {
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

// Set implements types.IPNIStore.
func (rs *RedisStore[Key, Value]) Set(ctx context.Context, key Key, value Value, expires bool) error {
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

// SetExpirable implements types.IPNIStore.
func (rs *RedisStore[Key, Value]) SetExpirable(ctx context.Context, key Key, expires bool) error {
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
