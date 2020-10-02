package trace

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"math/rand"
	"strconv"
	"time"
)

func MakeTrace() string {
	now := time.Now().UnixNano()
	encoder := md5.New()
	encoder.Write(strconv.AppendInt([]byte{}, rand.Int63(), 10))
	hash := hex.EncodeToString(encoder.Sum(nil))
	return fmt.Sprintf("%d|%s", now, hash)
}
func AppendTrace(origintrace, appname string) string {
	return fmt.Sprintf("%s|%s", origintrace, appname)
}
