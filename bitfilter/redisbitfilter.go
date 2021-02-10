package bitfilter

//import (
//        "context"
//        "crypto/sha1"
//        "encoding/hex"
//        "encoding/json"
//        "fmt"
//        "strconv"
//        "time"

//        "github.com/chenjie199234/Corelib/common"
//        "github.com/gomodule/redigo/redis"
//        "golang.org/x/sync/errgroup"
//)

////thread safe

//// RedisBitFilter -
//type RedisBitFilter struct {
//        pool      *redis.Pool
//        bitnum    uint64
//        groupnum  uint64
//        filterkey string
//        expire    int64
//}

//func init() {
//        //计算setlua脚本的hash值
//        h := sha1.Sum([]byte(setlua))
//        hsetlua = hex.EncodeToString(h[:])
//        //计算check脚本的hash值
//        h = sha1.Sum([]byte(checklua))
//        hchecklua = hex.EncodeToString(h[:])
//}

//var existlua = `local e=redis.call("GET",KEYS[1])
//if(e)
//then
//        return e
//end
//redis.call("SET",KEYS[1],ARGV[1],"EX",ARGV[2])`

//var initlua = `local e,k1,k2,k3,k4,k5,k6="{"..KEYS[1].."}e","{"..KEYS[1].."}1","{"..KEYS[1].."}2","{"..KEYS[1].."}3","{"..KEYS[1].."}4","{"..KEYS[1].."}5","{"..KEYS[1].."}6"
//if(redis.call("EXISTS",e)==0)
//then
//local r1=redis.call("SETBIT",k1,ARGV[1],1)
//local r2=redis.call("SETBIT",k2,ARGV[2],1)
//local r3=redis.call("SETBIT",k3,ARGV[3],1)
//local r4=redis.call("SETBIT",k4,ARGV[4],1)
//local r5=redis.call("SETBIT",k5,ARGV[5],1)
//local r6=redis.call("SETBIT",k6,ARGV[6],1)
//redis.call("SET",e,1,"EX",ARGV[7])
//redis.call("EXPIRE",k1,ARGV[7])
//redis.call("EXPIRE",k2,ARGV[7])
//redis.call("EXPIRE",k3,ARGV[7])
//redis.call("EXPIRE",k4,ARGV[7])
//redis.call("EXPIRE",k5,ARGV[7])
//redis.call("EXPIRE",k6,ARGV[7])
//end`

////默认占用60M内存
////单个hash列最大容量为8000w+
////6个hash列杂交,足以应对亿级的数据
////误判几率的公式推到为https://en.wikipedia.org/wiki/Bloom_filter
////以1亿key来计算,当第1亿零1个key插入时默认情况下的误判率大概为0.99%
//type Option struct {
//        //在cluster模式下,单个key会只存在某个固定的redis节点上
//        //因此为了防止该filter的所有的请求都打到同一redis节点上,可以将filter进行分组
//        //每组都是一个单独的filter[0-Groupnum),这样就能把请求分散到所有节点上
//        //默认该值为10
//        Groupnum uint64
//        //单位为byte
//        //每个group中hash列的最大容量
//        //单个hash列能表示的最大容量是Capacity * 8 * Groupnum
//        //每个filter的占用的内存为Capacity * 8 * hash_func_num(目前6个) * Groupnum
//        //默认该值为1024*1024(1M)
//        Capacity uint64
//        //该布隆过滤器的过期时间
//        //从New开始,持续Expire时间
//        //后续的操作不会更新生命周期,请合理设置生命周期
//        //最小1小时,默认为30天
//        Expire time.Duration
//}

//var defaultop = &Option{
//        Groupnum: 10,
//        Capacity: 1024 * 1024,
//        Expire:   30 * 24 * time.Hour,
//}

//func checkop(op *Option) {
//        if op.Groupnum == 0 {
//                //默认10
//                op.Groupnum = 10
//        }
//        if op.Capacity == 0 {
//                //默认1M
//                op.Capacity = 1024 * 1024
//        }
//        if op.Expire < time.Hour {
//                //默认30天
//                op.Expire = 30 * 24 * time.Hour
//        }
//}

//// ErrNoClient -
//var ErrNoClient = fmt.Errorf("missing redis client")

//// ErrNoKey -
//var ErrNoKey = fmt.Errorf("missing key")

//// ErrExist -
//var ErrExist = fmt.Errorf("this filter already exist,option conflict")

//// ErrKeyConflict
//var ErrKeyConflict = fmt.Errorf("this filter already used,key conflict")

//// NewRedisBitFilter -
////如果key之前使用过,而option也与之前使用过的一致,并且之前使用的key还未过期,那么将会复用之前创建的filter
////因此,建议,使用时请人工确保使用唯一key来当作key
//func NewRedisBitFilter(ctx context.Context, pool *redis.Pool, filterkey string, op *Option) (*RedisBitFilter, error) {
//        if pool == nil {
//                return nil, ErrNoClient
//        }
//        if filterkey == "" {
//                return nil, ErrNoKey
//        }
//        if op == nil {
//                op = defaultop
//        } else {
//                checkop(op)
//        }
//        //从redis中取该filter的配置
//        //如果redis中没有该filter的配置,表示是新创建
//        var exist bool = true
//        d, _ := json.Marshal(op)
//        c, e := pool.GetContext(ctx)
//        if e != nil {
//                return nil, e
//        }
//        opstr, e := redis.String(c.Do("EVAL", existlua, 1, filterkey, d, int64(op.Expire.Seconds())))
//        if e != nil {
//                if e == redis.ErrNil {
//                        exist = false
//                } else {
//                        c.Close()
//                        return nil, e
//                }
//        }
//        c.Close()
//        //如果已经存在了,那么使用已经存在option替代本次的option
//        if exist {
//                if e = json.Unmarshal([]byte(opstr), op); e != nil {
//                        //解析失败,只可能是key被别人使用了
//                        return nil, ErrKeyConflict
//                }
//                if op.Groupnum == 0 || op.Capacity == 0 || op.Expire < time.Hour {
//                        //解析失败,只可能是key被别人使用了
//                        return nil, ErrKeyConflict
//                }
//        }
//        //使用op进行初始化filter
//        instance := &RedisBitFilter{
//                pool:      pool,
//                bitnum:    op.Capacity * 8, //capacity的单位是byte,转换为bit需要乘8
//                groupnum:  op.Groupnum,
//                expire:    int64(op.Expire.Seconds()),
//                filterkey: filterkey,
//        }
//        //初始化redis内存,减少redis内存重分配
//        g, egctx := errgroup.WithContext(ctx)
//        for i := uint64(0); i < instance.groupnum; i++ {
//                tempindex := i
//                g.Go(func() error {
//                        key := instance.filterkey + strconv.FormatUint(tempindex, 10)
//                        bit := instance.bitnum
//                        c, e := pool.GetContext(egctx)
//                        if e != nil {
//                                return e
//                        }
//                        defer c.Close()
//                        _, e = c.Do("EVAL", initlua, 1, key, bit, bit, bit, bit, bit, bit, instance.expire)
//                        if e != nil && e != redis.ErrNil {
//                                return e
//                        }
//                        return nil
//                })
//        }
//        if e := g.Wait(); e != nil {
//                return nil, e
//        }
//        if exist {
//                if string(d) == opstr {
//                        //虽然是重复创建,但是option是一致的,所以可以创建成功
//                        return instance, nil
//                }
//                //option不一致的重复创建
//                return nil, ErrExist
//        }
//        return instance, nil
//}

//const setlua = `local e,k1,k2,k3,k4,k5,k6="{"..KEYS[1].."}e","{"..KEYS[1].."}1","{"..KEYS[1].."}2","{"..KEYS[1].."}3","{"..KEYS[1].."}4","{"..KEYS[1].."}5","{"..KEYS[1].."}6"
//if(redis.call("EXISTS",e)==0)
//then
//        return -1
//end
//local r1=redis.call("SETBIT",k1,ARGV[1],1)
//local r2=redis.call("SETBIT",k2,ARGV[2],1)
//local r3=redis.call("SETBIT",k3,ARGV[3],1)
//local r4=redis.call("SETBIT",k4,ARGV[4],1)
//local r5=redis.call("SETBIT",k5,ARGV[5],1)
//local r6=redis.call("SETBIT",k6,ARGV[6],1)
//if(redis.call("EXISTS",e)==0)
//then
//        redis.call("DEL",k1)
//        redis.call("DEL",k2)
//        redis.call("DEL",k3)
//        redis.call("DEL",k4)
//        redis.call("DEL",k5)
//        redis.call("DEL",k6)
//        return -1
//end
//if(r1==0 or r2==0 or r3==0 or r4==0 or r5==0 or r6==0)
//then
//        return 0
//end
//return 1`

//var hsetlua string

//// ErrExpired -
//var ErrExpired = fmt.Errorf("this filter expired")

//// Set 该方法会将key加入filter中,并判断key之前是否加入过
////error不为nil时表示出错
////error为nil时执行成功
////返回true,布隆过滤器中并没有该key,加入成功
////返回false,布隆过滤器中可能已经存在该key,无法100%保证,请进行其他额外逻辑处理
//func (b *RedisBitFilter) Set(ctx context.Context, key string) (bool, error) {
//        filterkey := b.getfilterkey(key)
//        bit1 := common.BkdrhashString(key, b.bitnum)
//        bit2 := common.DjbhashString(key, b.bitnum)
//        bit3 := common.FnvhashString(key, b.bitnum)
//        bit4 := common.DekhashString(key, b.bitnum)
//        bit5 := common.RshashString(key, b.bitnum)
//        bit6 := common.SdbmhashString(key, b.bitnum)
//        c, e := b.pool.GetContext(ctx)
//        if e != nil {
//                return false, e
//        }
//        defer c.Close()
//        //使用evalsha执行,节约带宽
//        r, e := redis.Int(c.Do("EVALSHA", hsetlua, 1, filterkey, bit1, bit2, bit3, bit4, bit5, bit6, b.expire))
//        if e != nil && e.Error() == "NOSCRIPT No matching script. Please use EVAL." {
//                //无脚本时改为使用eval安装脚本
//                r, e = redis.Int(c.Do("EVAL", setlua, 1, filterkey, bit1, bit2, bit3, bit4, bit5, bit6, b.expire))
//        }
//        if e != nil {
//                return false, e
//        }
//        if r == -1 {
//                return false, ErrExpired
//        }
//        return r == 0, nil
//}

//var checklua = `local e,k1,k2,k3,k4,k5,k6="{"..KEYS[1].."}e","{"..KEYS[1].."}1","{"..KEYS[1].."}2","{"..KEYS[1].."}3","{"..KEYS[1].."}4","{"..KEYS[1].."}5","{"..KEYS[1].."}6"
//if(redis.call("EXISTS",e)==0)
//then
//        return -1
//end
//local r1=redis.call("GETBIT",k1,ARGV[1])
//local r2=redis.call("GETBIT",k2,ARGV[2])
//local r3=redis.call("GETBIT",k3,ARGV[3])
//local r4=redis.call("GETBIT",k4,ARGV[4])
//local r5=redis.call("GETBIT",k5,ARGV[5])
//local r6=redis.call("GETBIT",k6,ARGV[6])
//if(redis.call("EXISTS",e)==0)
//then
//        redis.call("DEL",k1)
//        redis.call("DEL",k2)
//        redis.call("DEL",k3)
//        redis.call("DEL",k4)
//        redis.call("DEL",k5)
//        redis.call("DEL",k6)
//        return -1
//end
//if(r1==0 or r2==0 or r3==0 or r4==0 or r5==0 or r6==0)
//then
//        return 0
//end
//return 1`

//var hchecklua string

//// Check 该方法不会将key加入filter中,只会返回key是否不在filter中
//// 返回true,key确定100%不在filter中
//// 返回false,key不确定是否存在,需要其他额外逻辑辅助,非filter的逻辑
//func (b *RedisBitFilter) Check(ctx context.Context, key string) (bool, error) {
//        filterkey := b.getfilterkey(key)
//        bit1 := common.BkdrhashString(key, b.bitnum)
//        bit2 := common.DjbhashString(key, b.bitnum)
//        bit3 := common.FnvhashString(key, b.bitnum)
//        bit4 := common.DekhashString(key, b.bitnum)
//        bit5 := common.RshashString(key, b.bitnum)
//        bit6 := common.SdbmhashString(key, b.bitnum)
//        //使用evalsha执行,节约带宽
//        c, e := b.pool.GetContext(ctx)
//        if e != nil {
//                return false, e
//        }
//        defer c.Close()
//        r, e := redis.Int(c.Do("EVALSHA", hchecklua, 1, filterkey, bit1, bit2, bit3, bit4, bit5, bit6, b.expire))
//        if e != nil && e.Error() == "NOSCRIPT No matching script. Please use EVAL." {
//                //无脚本时改为使用eval安装脚本
//                r, e = redis.Int(c.Do("EVAL", checklua, 1, filterkey, bit1, bit2, bit3, bit4, bit5, bit6, b.expire))
//        }
//        if e != nil {
//                return false, e
//        }
//        if r == -1 {
//                return false, ErrExpired
//        }
//        return r == 0, nil
//}

//const cleanlua = `local e,k1,k2,k3,k4,k5,k6="{"..KEYS[1].."}e","{"..KEYS[1].."}1","{"..KEYS[1].."}2","{"..KEYS[1].."}3","{"..KEYS[1].."}4","{"..KEYS[1].."}5","{"..KEYS[1].."}6"
//redis.call("DEL",e)
//redis.call("DEL",k1)
//redis.call("DEL",k2)
//redis.call("DEL",k3)
//redis.call("DEL",k4)
//redis.call("DEL",k5)
//redis.call("DEL",k6)`

////only for test
//func (b *RedisBitFilter) clean() error {
//        c, e := b.pool.GetContext(context.Background())
//        if e != nil {
//                return e
//        }
//        defer c.Close()
//        for i := uint64(0); i < b.groupnum; i++ {
//                key := b.filterkey + strconv.FormatUint(i, 10)
//                _, e := c.Do("EVAL", cleanlua, 1, key)
//                if e != nil && e != redis.ErrNil {
//                        return e
//                }
//        }
//        if _, e = c.Do("DEL", b.filterkey); e != nil {
//                return e
//        }
//        return nil
//}

//func (b *RedisBitFilter) getfilterkey(userkey string) string {
//        return b.filterkey + strconv.FormatUint(common.BkdrhashString(userkey, uint64(b.groupnum)), 10)
//}
