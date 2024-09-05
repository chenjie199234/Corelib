package redis

import (
	"context"
	"log/slog"
	"math/rand"
	"strconv"
	"time"

	"github.com/chenjie199234/Corelib/util/common"

	gredis "github.com/redis/go-redis/v9"
)

var pubunicast *gredis.Script

func init() {
	pubunicast = gredis.NewScript(`redis.call("RPUSH",KEYS[1],unpack(ARGV))
redis.call("EXPIRE",KEYS[1],16)
return #ARGV`)
}

// SubUnicast will sub max 128 datas in one cycle
// last: wether this is the last data in this cycle
type UnicastHandler func(data []byte, last bool)

// due to go-redis doesn't support to wake up the block cmd actively now
// so the stop func can't stop the sub when there is not data in the list(cmd is blocked)
// shard: is used to split data into different redis lists
func (c *Client) SubUnicast(unicast string, shard uint8, handler UnicastHandler) (stop func()) {
	if unicast == "" || shard == 0 {
		panic("[redis.unicast.sub] unicast name or shard num missing")
	}
	ctx, cancel := context.WithCancel(context.Background())
	for i := uint8(0); i < shard; i++ {
		list := "unicast_" + unicast + "_" + strconv.Itoa(int(i))
		go func() {
			time.Sleep(time.Duration(rand.Int63n(time.Second.Nanoseconds() * 5)))
			tker := time.NewTicker(time.Second * 5)
			for {
				select {
				case <-ctx.Done():
					tker.Stop()
					return
				case <-tker.C:
					if _, err := c.Expire(ctx, list, time.Second*16).Result(); err != nil {
						if err == context.Canceled {
							continue
						}
						if err == gredis.ErrClosed {
							tker.Stop()
							return
						}
						slog.ErrorContext(ctx, "[redis.unicast.sub] expire failed", slog.String("list", list), slog.String("error", err.Error()))
					}
				}
			}
		}()
		go func() {
			for {
				_, rs, err := c.BLMPop(ctx, 0, "LEFT", 128, list).Result()
				if err != nil {
					if err == gredis.ErrClosed || err == context.Canceled {
						return
					}
					slog.ErrorContext(ctx, "[redis.unicast.sub] read failed", slog.String("list", list), slog.String("error", err.Error()))
					time.Sleep(time.Millisecond * 100)
					continue
				}
				for i, r := range rs {
					handler(common.STB(r), i == len(rs)-1)
				}
			}
		}()
	}
	return cancel
}

// shard: is used to split data into different redis lists
// key: is only used to shard values into different redis node(hash),if key is empty,values will be sharded randomly
// return gredis.Nil means the unicast doesn't exist
func (c *Client) PubUnicast(ctx context.Context, unicast string, shard uint8, key string, values ...interface{}) error {
	if unicast == "" || shard == 0 {
		panic("[redis.unicast.pub] unicast name or shard num missing")
	}
	list := ""
	if key == "" {
		list = "unicast_" + unicast + "_" + strconv.FormatUint(rand.Uint64()%uint64(shard), 10)
	} else {
		list = "unicast_" + unicast + "_" + strconv.FormatUint(common.Bkdrhash(common.STB(key), uint64(shard)), 10)
	}
	return pubunicast.Run(ctx, c, []string{list}, values...).Err()
}
