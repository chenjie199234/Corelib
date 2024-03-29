package discover

import (
	"testing"
	"time"
)

func Test_Discover(t *testing.T) {
	d, e := NewDNSDiscover("testp", "testg", "testn", "www.baidu.com", time.Second*10, 9000, 10000, 8000)
	if e != nil {
		t.Fatal("new dns discover error:" + e.Error())
	}
	go func() {
		ch, cancel := d.GetNotice()
		defer cancel()
		for {
			<-ch
			addrs, version, e := d.GetAddrs(Web)
			t.Logf("addrs:%v\n", addrs)
			t.Logf("version:%v\n", version)
			t.Logf("lasterror:%v\n", e)
		}
	}()
	go func() {
		time.Sleep(time.Second)
		d.Now()
		time.Sleep(time.Second)
		d.Now()
		time.Sleep(time.Second)
		d.Now()
	}()
	select {}
}
