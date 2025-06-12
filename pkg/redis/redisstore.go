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

// Client is a subset of functions from the golang redis client that we need to implement our cache
type Client interface {
	Get(context.Context, string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	SAdd(ctx context.Context, key string, members ...interface{}) *redis.IntCmd
	SMembers(ctx context.Context, key string) *redis.StringSliceCmd
	Expire(ctx context.Context, key string, expiration time.Duration) *redis.BoolCmd
	Persist(ctx context.Context, key string) *redis.BoolCmd
}

// Pipelineable allows pipelines to be created for batching of write commands.
type Pipelineable interface {
	Pipeline() Pipeliner
}

// Pipeliner is a subset of functions from [redis.Pipeliner] that we need to
// implement pipelining for our cache.
type Pipeliner interface {
	SAdd(ctx context.Context, key string, members ...any) *redis.IntCmd
	Expire(ctx context.Context, key string, expiration time.Duration) *redis.BoolCmd
	Persist(ctx context.Context, key string) *redis.BoolCmd
	Exec(ctx context.Context) ([]redis.Cmder, error)
}

// PipelineClient is a client that also supports pipelining.
type PipelineClient interface {
	Client
	Pipelineable
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
		duration = rs.config.expirationTime
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

type BatchingValueSetStore[K, V any] struct {
	store     *Store[K, V]
	client    PipelineClient
	toRedis   func(V) (string, error)
	keyString func(K) string
	opts      []Option
}

// NewBatchingValueSetStore creates a new value-set store (a store whose values
// are sets) that allows batching.
func NewBatchingValueSetStore[K, V any](
	fromRedis func(string) (V, error),
	toRedis func(V) (string, error),
	keyString func(K) string,
	client PipelineClient,
	opts ...Option,
) *BatchingValueSetStore[K, V] {
	return &BatchingValueSetStore[K, V]{
		store:     NewStore(fromRedis, toRedis, keyString, client, opts...),
		client:    client,
		toRedis:   toRedis,
		keyString: keyString,
		opts:      opts,
	}
}

func (bvs *BatchingValueSetStore[K, V]) Add(ctx context.Context, key K, values ...V) (uint64, error) {
	return bvs.store.Add(ctx, key, values...)
}

func (bvs *BatchingValueSetStore[K, V]) SetExpirable(ctx context.Context, key K, expires bool) error {
	return bvs.store.SetExpirable(ctx, key, expires)
}

func (bvs *BatchingValueSetStore[K, V]) Members(ctx context.Context, key K) ([]V, error) {
	return bvs.store.Members(ctx, key)
}

func (bvs *BatchingValueSetStore[K, V]) Batch() types.ValueSetCacheBatcher[K, V] {
	return NewPipelineBatcher(bvs.client.Pipeline(), bvs.toRedis, bvs.keyString, bvs.opts...)
}

type PipelineBatcher[K, V any] struct {
	toRedis   func(V) (string, error)
	keyString func(K) string
	config    config
	pipeline  Pipeliner
}

func NewPipelineBatcher[K, V any](
	pipeline Pipeliner,
	toRedis func(V) (string, error),
	keyString func(K) string,
	opts ...Option,
) *PipelineBatcher[K, V] {
	batcher := PipelineBatcher[K, V]{
		toRedis:   toRedis,
		keyString: keyString,
		config:    newConfig(opts),
		pipeline:  pipeline,
	}
	return &batcher
}

func (pb *PipelineBatcher[K, V]) Add(ctx context.Context, key K, values ...V) error {
	var data []any
	for _, v := range values {
		d, err := pb.toRedis(v)
		if err != nil {
			return err
		}
		data = append(data, d)
	}
	pb.pipeline.SAdd(ctx, pb.keyString(key), data...)
	return nil
}

func (pb *PipelineBatcher[K, V]) SetExpirable(ctx context.Context, key K, expires bool) error {
	if expires {
		pb.pipeline.Expire(ctx, pb.keyString(key), pb.config.expirationTime)
	} else {
		pb.pipeline.Persist(ctx, pb.keyString(key))
	}
	return nil
}

func (pb *PipelineBatcher[K, V]) Commit(ctx context.Context) error {
	_, err := pb.pipeline.Exec(ctx)
	return err
}

// NewClientAdapter converts a [redis.Client] into a [PipelineClient].
func NewClientAdapter(client *redis.Client) PipelineClient {
	return &clientAdapter{client}
}

type clientAdapter struct {
	client *redis.Client
}

func (a *clientAdapter) Expire(ctx context.Context, key string, expiration time.Duration) *redis.BoolCmd {
	return a.client.Expire(ctx, key, expiration)
}

func (a *clientAdapter) Get(ctx context.Context, key string) *redis.StringCmd {
	return a.client.Get(ctx, key)
}

func (a *clientAdapter) Persist(ctx context.Context, key string) *redis.BoolCmd {
	return a.client.Persist(ctx, key)
}

func (a *clientAdapter) Pipeline() Pipeliner {
	return a.client.Pipeline()
}

func (a *clientAdapter) SAdd(ctx context.Context, key string, members ...interface{}) *redis.IntCmd {
	return a.client.SAdd(ctx, key, members...)
}

func (a *clientAdapter) SMembers(ctx context.Context, key string) *redis.StringSliceCmd {
	return a.client.SMembers(ctx, key)
}

func (a *clientAdapter) Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd {
	return a.client.Set(ctx, key, value, expiration)
}

var _ PipelineClient = (*clientAdapter)(nil)

// NewClientAdapter converts a [redis.ClusterClient] into a [PipelineClient].
func NewClusterClientAdapter(client *redis.ClusterClient) PipelineClient {
	return &clusterClientAdapter{client}
}

type clusterClientAdapter struct {
	client *redis.ClusterClient
}

func (a *clusterClientAdapter) Expire(ctx context.Context, key string, expiration time.Duration) *redis.BoolCmd {
	return a.client.Expire(ctx, key, expiration)
}

func (a *clusterClientAdapter) Get(ctx context.Context, key string) *redis.StringCmd {
	return a.client.Get(ctx, key)
}

func (a *clusterClientAdapter) Persist(ctx context.Context, key string) *redis.BoolCmd {
	return a.client.Persist(ctx, key)
}

func (a *clusterClientAdapter) Pipeline() Pipeliner {
	return a.client.Pipeline()
}

func (a *clusterClientAdapter) SAdd(ctx context.Context, key string, members ...interface{}) *redis.IntCmd {
	return a.client.SAdd(ctx, key, members...)
}

func (a *clusterClientAdapter) SMembers(ctx context.Context, key string) *redis.StringSliceCmd {
	return a.client.SMembers(ctx, key)
}

func (a *clusterClientAdapter) Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd {
	return a.client.Set(ctx, key, value, expiration)
}

var _ PipelineClient = (*clientAdapter)(nil)
