package nobody

import (
	"math/rand"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func Benchmark_nobody_router(b *testing.B) {
	b.StopTimer()
	rand.Seed(time.Now().Unix())
	r := new(Router)
	r.init()
	urls := makeurls("ac", "de", "mzf")
	fn := func(c *Context) {}
	for _, url := range urls {
		r.Get(url, fn)
	}
	ctx := &Context{
		request: &http.Request{
			Method: "GET",
			URL:    &url.URL{Path: ""},
		},
		params: make(map[string]string),
	}
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		ctx.request.URL.Path = urls[rand.Int()%len(urls)]
		r.search(ctx)
		for _, call := range ctx.calls {
			call(ctx)
		}
	}
}
func Benchmark_default_router(b *testing.B) {
	b.StopTimer()
	rand.Seed(time.Now().Unix())
	mux := http.NewServeMux()
	urls := makeurls("ac", "de", "mzf")
	fn := func(http.ResponseWriter, *http.Request) {}
	for _, url := range urls {
		mux.HandleFunc(url, fn)
	}
	r := &http.Request{
		URL: &url.URL{
			Scheme: "http",
			Host:   "127.00.0.1:8080",
			Path:   "",
		},
		Host: "127.00.0.1:8080",
	}
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		r.URL.Path = urls[rand.Int()%len(urls)]
		f, _ := mux.Handler(r)
		if f != nil {
			f.ServeHTTP(nil, nil)
		}
	}
}
func makeurls(key1, key2, key3 string) []string {
	k1 := randKey(key1)
	k2 := randKey(key2)
	k3 := randKey(key3)
	urls := make([]string, 0)
	for a := 0; a < len(k1); a++ {
		for b := 0; b < len(k2); b++ {
			for c := 0; c < len(k3); c++ {
				urls = append(urls, "/"+k1[a]+"/"+k2[b]+"/"+k3[c])
			}
		}
	}
	return urls
}
func randKey(key string) []string {
	tempkey := make([]byte, len(key))
	tempkeys := make([]string, 0)
	for i := 0; i < (1 << uint(len(key))); i++ {
		for j := 0; j < len(key); j++ {
			if (i & (1 << uint(j))) > 0 {
				if key[j] >= 96 {
					tempkey[j] = key[j] - 32
				} else {
					tempkey[j] = key[j]
				}
			} else {
				if key[j] >= 96 {
					tempkey[j] = key[j]
				} else {
					tempkey[j] = key[j] + 32
				}
			}
		}
		tempkeys = append(tempkeys, string(tempkey))
	}
	return tempkeys
}
