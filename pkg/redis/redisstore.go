package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/storacha/go-libstoracha/jobqueue"
	"github.com/storacha/indexing-service/pkg/types"
)

// DefaultExpire is the expire time we set on Redis when Set/SetExpiration are called with expire=true
const DefaultExpire = time.Hour

// Client is a subset of functions from the golang redis client that we need to implement our cache
type Client interface {
	Get(context.Context, string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	SAdd(ctx context.Context, key string, members ...interface{}) *redis.IntCmd
	SMembers(ctx context.Context, key string) *redis.StringSliceCmd
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
	config    config
}

var (
	_ Client                = (*redis.Client)(nil)
	_ types.Cache[any, any] = (*Store[any, any])(nil)
)

type config struct {
	expirationTime time.Duration
}

func newConfig(opts []Option) config {
	c := config{
		expirationTime: DefaultExpire,
	}
	for _, opt := range opts {
		opt(&c)
	}
	return c
}

type Option func(*config)

func ExpirationTime(expirationTime time.Duration) Option {
	return func(c *config) {
		c.expirationTime = expirationTime
	}
}

// NewStore returns a new instance of a redis store with the provided serialization/deserialization functions
func NewStore[Key, Value any](
	fromRedis func(string) (Value, error),
	toRedis func(Value) (string, error),
	keyString func(Key) string,
	client Client,
	opts ...Option) *Store[Key, Value] {
	return &Store[Key, Value]{fromRedis, toRedis, keyString, client, newConfig(opts)}
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
		err = rs.client.Expire(ctx, rs.keyString(key), rs.config.expirationTime).Err()
	} else {
		err = rs.client.Persist(ctx, rs.keyString(key)).Err()
	}
	if err != nil {
		return fmt.Errorf("error accessing redis: %w", err)
	}
	return nil
}

// Members returns all deserialized set values from redis.
// If the key does not exist, it returns ErrKeyNotFound.
func (rs *Store[Key, Value]) Members(ctx context.Context, key Key) ([]Value, error) {
	data, err := rs.client.SMembers(ctx, rs.keyString(key)).Result()
	if err != nil {
		return nil, fmt.Errorf("getting set members: %w", err)
	}

	// as opposed to other commands, SMembers doesn't return redis.Nil when the key doesn't exist, but an empty set
	// this implementation assumes there is no need to differentiate between a non-existing key and an empty set
	if len(data) == 0 {
		return nil, types.ErrKeyNotFound
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
func (rs *Store[Key, Value]) Add(ctx context.Context, key Key, values ...Value) (uint64, error) {
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

type operation[Value any] struct {
	values  []Value
	expires *bool
}

func (o *operation[Value]) Add(values ...Value) {
	o.values = append(o.values, values...)
}

func (o *operation[Value]) SetExpirable(expires bool) {
	o.expires = &expires
}

func (rs *Store[Key, Value]) MultiAdd(ctx context.Context, keys []Key, setupOperation func(types.MultiOperation[Value])) (uint64, error) {
	o := operation[Value]{}
	setupOperation(&o)
	var data []any
	for _, v := range o.values {
		d, err := rs.toRedis(v)
		if err != nil {
			return 0, err
		}
		data = append(data, d)
	}

	var joberr error
	q := jobqueue.NewJobQueue[Key](
		jobqueue.JobHandler(func(ctx context.Context, key Key) error {
			if len(data) > 0 {
				_, err := rs.client.SAdd(ctx, rs.keyString(key), data...).Result()
				if err != nil {
					return fmt.Errorf("adding set member: %w", err)
				}
			}
			if o.expires == nil {
				return nil
			}
			return rs.SetExpirable(ctx, key, *o.expires)
		}),
		jobqueue.WithConcurrency(5),
		jobqueue.WithErrorHandler(func(err error) { joberr = err }),
	)
	q.Startup()
	i := uint64(0)
	for _, key := range keys {
		err := q.Queue(ctx, key)
		if err != nil {
			return i, err
		}
		i++
	}
	err := q.Shutdown(ctx)
	if err != nil {
		return i, fmt.Errorf("shutting down job queue: %w", err)
	}
	if joberr != nil {
		return i, fmt.Errorf("executing multi store operation: %w", joberr)
	}
	return i, nil
}
