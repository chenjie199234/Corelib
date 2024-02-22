package redis

import (
	"context"
	"math/rand"
	"strconv"
	"sync"
	"time"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/common"

	gredis "github.com/redis/go-redis/v9"
)

// due to go-redis doesn't support to wake up the block cmd actively now,so the stop func can't stop the read now.
// shard: is used to split data into different redis streams
func (c *Client) SubBroadcast(broadcast string, shard uint8, handler func(values map[string]interface{})) (stop func(), e error) {
	if broadcast == "" || shard == 0 {
		panic("[redis.broadcast.sub] broadcast name or shard num missing")
	}
	ctx, cancel := context.WithCancel(context.Background())
	for i := uint8(0); i < shard; i++ {
		stream := "broadcast_" + broadcast + "_" + strconv.Itoa(int(i))
		go func() {
			id := "$"
			for {
				rs, err := c.XRead(ctx, &gredis.XReadArgs{
					Streams: []string{stream, id},
					Count:   32,
					Block:   0,
				}).Result()
				if err != nil {
					if err == gredis.ErrClosed || err == context.Canceled {
						return
					}
					log.Error(ctx, "[redis.broadcast.sub] read failed", log.String("stream", stream))
					time.Sleep(time.Millisecond * 100)
					continue
				}
				for _, r := range rs {
					for _, m := range r.Messages {
						id = m.ID
						handler(m.Values)
					}
				}
			}
		}()
	}
	return func() { cancel() }, nil
}

// shard: is used to split data into different redis streams
// key: is only used to shard values into different redis node(hash),if key is empty,values will be sharded randomly
func (c *Client) PubBroadcast(ctx context.Context, broadcast string, shard uint8, key string, values map[string]interface{}) error {
	if broadcast == "" || shard == 0 {
		panic("[redis.broadcast.pub] broadcast name or shard num missing")
	}
	stream := ""
	if key == "" {
		stream = "broadcast_" + broadcast + "_" + strconv.FormatUint(rand.Uint64()%uint64(shard), 10)
	} else {
		stream = "broadcast_" + broadcast + "_" + strconv.FormatUint(common.Bkdrhash(common.STB(key), uint64(shard)), 10)
	}
	args := &gredis.XAddArgs{
		Stream: stream,
		Values: values,
	}
	_, e := c.XAdd(ctx, args).Result()
	return e
}

// shard: is used to split data into different redis streams
func (c *Client) DelBroadcast(ctx context.Context, broadcast string, shard uint8) (e error) {
	if broadcast == "" || shard == 0 {
		panic("[redis.broadcast.del] broadcast name or shard num missing")
	}
	wg := sync.WaitGroup{}
	for i := uint8(0); i < shard; i++ {
		stream := "broadcast_" + broadcast + "_" + strconv.Itoa(int(i))
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Del(ctx, stream).Result(); err != nil {
				e = err
			}
		}()
	}
	wg.Wait()
	return
}

// shard: is used to split data into different redis streams
// timestamp: unit seconds,all messages before this timestamp will be deleted
func (c *Client) TrimBroadcast(ctx context.Context, broadcast string, shard uint8, timestamp uint64) (e error) {
	if broadcast == "" || shard == 0 {
		panic("[redis.broadcast.trim] broadcast name or shard num missing")
	}
	wg := sync.WaitGroup{}
	for i := uint8(0); i < shard; i++ {
		stream := "broadcast_" + broadcast + "_" + strconv.Itoa(int(i))
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.XTrimMinID(ctx, stream, strconv.FormatUint(timestamp*1000, 10)+"-0").Result(); err != nil {
				e = err
			}
		}()
	}
	wg.Wait()
	return
}
