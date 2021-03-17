package common

import (
	"bytes"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
)

func Test_Nil(t *testing.T) {
	a := ([]byte)(nil)
	if Byte2str(a) != "" {
		panic("nil")
	}
	b := make([]byte, 0)
	if Byte2str(b) != "" {
		panic("empty")
	}
}
func Test_Common(t *testing.T) {
	var sempty string = ""
	var s string = "abc"
	var bempty []byte = nil
	var b []byte = []byte{'a', 'b', 'c'}

	if !bytes.Equal(Str2byte(s), b) {
		panic("not empty str to byte failed")
	}
	if !bytes.Equal(Str2byte(sempty), bempty) {
		panic("empty str to byte failed")
	}

	if Byte2str(b) != s {
		panic("not empty byte to str failed")
	}
	if Byte2str(bempty) != sempty {
		panic("empty byte to str failed")
	}
	rand.Seed(time.Now().UnixNano())
	wg := &sync.WaitGroup{}
	wg.Add(6)
	go func() {
		defer wg.Done()
		fmt.Println("bkdr: ", testhash(BkdrhashString, BkdrhashByte))
	}()
	go func() {
		defer wg.Done()
		fmt.Println("fnv:  ", testhash(FnvhashString, FnvhashByte))
	}()
	go func() {
		defer wg.Done()
		fmt.Println("rs:   ", testhash(RshashString, RshashByte))
	}()
	go func() {
		defer wg.Done()
		fmt.Println("sdbm: ", testhash(SdbmhashString, SdbmhashByte))
	}()
	go func() {
		defer wg.Done()
		fmt.Println("dek:  ", testhash(DekhashString, DekhashByte))
	}()
	go func() {
		defer wg.Done()
		fmt.Println("djb:  ", testhash(DjbhashString, DjbhashByte))
	}()
	wg.Wait()
}

//capacity 1024*1024*8
//test data 1024*1024*4
func testhash(strf func(string, uint64) uint64, bytef func([]byte, uint64) uint64) int {
	total := 1024 * 1024 * 8
	conflict := 0
	bit := make(map[uint64]struct{}, total)
	for i := 0; i < total/2; i++ {
		keyb := makerandkeybyte()
		keys := Byte2str(keyb)
		indexs := strf(keys, uint64(total))
		indexb := bytef(keyb, uint64(total))
		if indexs != indexb {
			panic("string hash caculate different with byteslice hash")
		}
		if _, ok := bit[indexs]; ok {
			conflict++
		} else {
			bit[indexs] = struct{}{}
		}
	}
	return conflict
}

//key contain [0-9][a-z][A-Z][_:-|]
func makerandkeystr() string {
	return Byte2str(makerandkeybyte())
}
func makerandkeybyte() []byte {
	result := make([]byte, 10)
	for i := 0; i < 10; i++ {
		switch rand.Intn(4) {
		case 0:
			result[i] = byte(rand.Intn(10) + 48)
		case 1:
			result[i] = byte(rand.Intn(26) + 97)
		case 2:
			result[i] = byte(rand.Intn(26) + 65)
		case 3:
			switch rand.Intn(4) {
			case 0:
				result[i] = '_'
			case 1:
				result[i] = ':'
			case 2:
				result[i] = '-'
			case 3:
				result[i] = '|'
			}
		}
	}
	return result
}
