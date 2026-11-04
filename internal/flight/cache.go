package flight

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	flightKeyPrefix = "flight:"
	searchKeyPrefix = "search:"
	cacheTTL        = 5 * time.Minute
)

type Cache struct {
	client *redis.Client
	ttl    time.Duration
}

func NewCache(sentinelAddrs []string, masterName string) *Cache {
	return &Cache{
		client: redis.NewFailoverClient(&redis.FailoverOptions{
			MasterName:    masterName,
			SentinelAddrs: sentinelAddrs,
		}),
		ttl: cacheTTL,
	}
}

func (c *Cache) Close() error {
	return c.client.Close()
}

func (c *Cache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *Cache) GetFlight(ctx context.Context, id string) (Flight, bool, error) {
	key := flightKey(id)
	raw, err := c.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		log.Printf("cache miss flight %s", id)
		return Flight{}, false, nil
	}
	if err != nil {
		return Flight{}, false, err
	}

	var f Flight
	if err := json.Unmarshal(raw, &f); err != nil {
		_ = c.client.Del(ctx, key)
		return Flight{}, false, err
	}
	log.Printf("cache hit flight %s", id)
	return f, true, nil
}

func (c *Cache) SetFlight(ctx context.Context, f Flight) error {
	raw, err := json.Marshal(f)
	if err != nil {
		return err
	}
	if err := c.client.Set(ctx, flightKey(f.ID), raw, c.ttl).Err(); err != nil {
		return err
	}
	log.Printf("cache set flight %s ttl %s", f.ID, c.ttl)
	return nil
}

func (c *Cache) GetSearch(ctx context.Context, origin, destination string, date *time.Time) ([]Flight, bool, error) {
	key := searchKey(origin, destination, date)
	raw, err := c.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		log.Printf("cache miss search %s", key)
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	var flights []Flight
	if err := json.Unmarshal(raw, &flights); err != nil {
		_ = c.client.Del(ctx, key)
		return nil, false, err
	}
	log.Printf("cache hit search %s", key)
	return flights, true, nil
}

func (c *Cache) SetSearch(ctx context.Context, origin, destination string, date *time.Time, flights []Flight) error {
	raw, err := json.Marshal(flights)
	if err != nil {
		return err
	}
	key := searchKey(origin, destination, date)
	if err := c.client.Set(ctx, key, raw, c.ttl).Err(); err != nil {
		return err
	}
	log.Printf("cache set search %s ttl %s", key, c.ttl)
	return nil
}

func (c *Cache) InvalidateFlight(ctx context.Context, id string) error {
	deleted, err := c.client.Del(ctx, flightKey(id)).Result()
	if err != nil {
		return err
	}
	log.Printf("cache invalidate flight %s deleted %d", id, deleted)
	return nil
}

func (c *Cache) InvalidateSearch(ctx context.Context) error {
	var cursor uint64
	var deleted int64
	for {
		keys, next, err := c.client.Scan(ctx, cursor, searchKeyPrefix+"*", 100).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			count, err := c.client.Del(ctx, keys...).Result()
			if err != nil {
				return err
			}
			deleted += count
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	log.Printf("cache invalidate search deleted %d", deleted)
	return nil
}

func flightKey(id string) string {
	return flightKeyPrefix + id
}

func searchKey(origin, destination string, date *time.Time) string {
	day := "any"
	if date != nil {
		day = date.Format("2006-01-02")
	}
	return fmt.Sprintf("%s%s:%s:%s", searchKeyPrefix, origin, destination, day)
}
