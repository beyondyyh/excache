package excache

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLRUCache(t *testing.T) {
	var ok bool
	// Test wrong params
	func() {
		defer func() {
			r := recover()
			assert.NotNil(t, r)
			assert.Equal(t, "Size of LRUCache should not less than zero", r.(string))
		}()
		NewLRUCache(-1, 1, time.Second)
	}()
	func() {
		defer func() {
			r := recover()
			assert.NotNil(t, r)
			assert.Equal(t, "Used-Count-To-Live of LRUCache should not less than zero", r.(string))
		}()
		NewLRUCache(1, -1, time.Second)
	}()
	func() {
		defer func() {
			r := recover()
			assert.NotNil(t, r)
			assert.Equal(t, "Time-To-Live of LRUCache should not less than zero", r)
		}()
		NewLRUCache(1, 1, -1*time.Second)
	}()
	// Prepare
	cache := NewLRUCache(3, 2, 1*time.Second)
	for i := 0; i < 4; i++ {
		cache.Set(fmt.Sprintf("mykey_%d", i), "useless value")
	}

	_, ok = cache.Get("nokey")
	assert.False(t, ok)

	cache.Set("mykey", []string{"hello", "goodbye"})
	cache.Set("mykey", []string{"hello", "goodbye"})
	// parallel get to check use count to live
	uc := 0
	sig := make(chan struct{})
	ucf := func() {
		if _, ok = cache.Get("mykey"); ok {
			uc++
		}
		sig <- struct{}{}
	}
	for i := 0; i < 3; i++ {
		go ucf()
	}
	for i := 0; i < 3; i++ {
		<-sig
	}
	assert.Equal(t, 2, uc)

	// Test TTL
	time.Sleep(1 * time.Second)
	_, ok = cache.Get("mykey_3")
	assert.False(t, ok)

	// Test Count
	i, q, h := cache.Count()
	assert.Equal(t, uint64(1), i) // mykey and mykey_3 is removed by Get, but mykey_2 remain
	assert.Equal(t, uint64(5), q)
	assert.Equal(t, uint64(2), h)

	// Test SetExTTL
	cache.SetExTTL(time.Second * 3)
	cache.Set("mykey_4", 1)
	time.Sleep(2 * time.Second)
	v, ok := cache.Get("mykey_4")
	assert.NotNil(t, v)
	assert.Equal(t, 1, v)

	cache.Purge()
}

func BenchmarkLRUCache(b *testing.B) {
	cache := NewLRUCache(2000, 1, 1*time.Second)
	var wg sync.WaitGroup
	for i := 0; i < b.N; i++ {
		wg.Add(1)
		key := fmt.Sprintf("key_%d", i)
		go func() {
			cache.Set(key, "some value")
			cache.Get(key)
			wg.Done()
		}()
	}
	wg.Wait()
}
