package redis

import (
	"context"
	"math/rand"
	"strconv"
	"time"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/common"

	gredis "github.com/redis/go-redis/v9"
)

var pubunicast *gredis.Script

func init() {
	pubunicast = gredis.NewScript(`if(redis.call("EXISTS",KEYS[2])==0)
then
	return nil
end
redis.call("EXPIRE",KEYS[1],16)
redis.call("RPUSH",KEYS[1],unpack(ARGV))
return #ARGV`)
}

// due to go-redis doesn't support to wake up the block cmd actively now,so the stop func can't stop the sub now,but it try to prevent the new pub
// shard: is used to split data into different redis lists
func (c *Client) SubUnicast(unicast string, shard uint8, handler func([]byte)) (stop func(), e error) {
	if unicast == "" || shard == 0 {
		panic("[redis.unicast.sub] unicast name or shard num missing")
	}
	ctx, cancel := context.WithCancel(context.Background())
	for i := uint8(0); i < shard; i++ {
		list := "unicast_" + unicast + "_" + strconv.Itoa(int(i))
		exist := "{" + list + "}_exist"
		go func() {
			for {
				if _, err := c.SetEx(ctx, exist, 1, time.Second*16).Result(); err != nil {
					log.Error(ctx, "[redis.unicast.sub] init failed", log.String("list", list), log.CError(err))
					time.Sleep(time.Millisecond * 100)
					continue
				}
				break
			}
			go func() {
				time.Sleep(time.Duration(rand.Int63n(time.Second.Nanoseconds() * 5)))
				tker := time.NewTicker(time.Second * 5)
				for {
					select {
					case <-ctx.Done():
						if _, err := c.Del(context.Background(), exist).Result(); err != nil {
							log.Error(ctx, "[redis.unicast.sub] stop failed", log.String("list", list), log.CError(err))
						}
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
							log.Error(ctx, "[redis.unicast.sub] expire failed", log.String("list", list), log.CError(err))
						}
						if _, err := c.SetEx(ctx, exist, 1, time.Second*16).Result(); err != nil {
							if err == context.Canceled {
								continue
							}
							if err == gredis.ErrClosed {
								tker.Stop()
								return
							}
							log.Error(ctx, "[redis.unicast.sub] expire failed", log.String("list", list), log.CError(err))
						}
					}
				}
			}()
			go func() {
				for {
					r, err := c.BLPop(ctx, 0, list).Result()
					if err != nil {
						if err == gredis.ErrClosed || err == context.Canceled {
							return
						}
						log.Error(ctx, "[redis.unicast.sub] read failed", log.String("list", list), log.CError(err))
						time.Sleep(time.Millisecond * 100)
						continue
					}
					handler(common.STB(r[1]))
				}
			}()
		}()
	}
	return func() { cancel() }, nil
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
	exist := "{" + list + "}_exist"
	return pubunicast.Run(ctx, c, []string{list, exist}, values...).Err()
}
