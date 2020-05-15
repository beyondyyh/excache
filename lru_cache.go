package excache

import (
	"container/list"
	"sync"
	"time"
)

// LRUCache a cache library with LRU
type LRUCache struct {
	lruMap   map[interface{}]*item
	lruList  *list.List
	ttl      time.Duration // time-to-live
	uctl     int           // used-count-to-live
	size     int
	m        sync.RWMutex
	cntQuery uint64
	cntHit   uint64
}

type item struct {
	el  *list.Element
	uc  int       // used count
	et  time.Time // expired time
	val interface{}
}

// NewLRUCache create a new LRUCache instance with size=size. Age means
// how much times an item can be used, and expire means how long time an
// item can be used. 0 Age or 0 expire means no limitation.
func NewLRUCache(size int, uctl int, ttl time.Duration) *LRUCache {
	if size < 0 {
		panic("Size of LRUCache should not less than zero")
	}
	if uctl < 0 {
		panic("Used-Count-To-Live of LRUCache should not less than zero")
	}
	if ttl < 0 {
		panic("Time-To-Live of LRUCache should not less than zero")
	}
	return &LRUCache{
		lruMap:  make(map[interface{}]*item, size),
		lruList: list.New(),
		size:    size,
		uctl:    uctl,
		ttl:     ttl,
	}
}

// SetExTTL Update the ttl for subsequent new item
func (lc *LRUCache) SetExTTL(t time.Duration) {
	lc.m.Lock()
	lc.ttl = t
	lc.m.Unlock()
}

// SetExUCTL Update the uctl for subsequent new item
func (lc *LRUCache) SetExUCTL(ut int) {
	lc.m.Lock()
	lc.uctl = ut
	lc.m.Unlock()
}

// Get get item from cache
func (lc *LRUCache) Get(key interface{}) (interface{}, bool) {
	lc.m.Lock()
	defer lc.m.Unlock()

	lc.cntQuery++
	if v, ok := lc.lruMap[key]; ok {
		// check age
		if lc.uctl > 0 {
			if v.uc >= lc.uctl {
				delete(lc.lruMap, key)
				lc.lruList.Remove(v.el)
				return nil, false
			}
			v.uc++
		}
		// check expiration
		if lc.ttl > 0 {
			if v.et.Before(time.Now()) {
				delete(lc.lruMap, key)
				lc.lruList.Remove(v.el)
				return nil, false
			}
		}
		// hit
		lc.lruList.MoveToFront(v.el)
		lc.cntHit++
		return v.val, true
	}
	return nil, false
}

// Set set item into cache
func (lc *LRUCache) Set(key, val interface{}) {
	lc.m.Lock()
	defer lc.m.Unlock()

	if v, ok := lc.lruMap[key]; ok {
		v.val = val
		v.uc = 0
		v.et = time.Now().Add(lc.ttl)
		lc.lruList.MoveToFront(v.el)
		return
	}

	el := lc.lruList.PushFront(key)
	lc.lruMap[key] = &item{
		val: val,
		uc:  0,
		et:  time.Now().Add(lc.ttl),
		el:  el,
	}

	for lc.lruList.Len() > lc.size {
		last := lc.lruList.Back()
		delete(lc.lruMap, last.Value)
		lc.lruList.Remove(last)
	}
}

// Purge remove all cache
func (lc *LRUCache) Purge() {
	lc.m.Lock()
	defer lc.m.Unlock()

	lc.lruMap = make(map[interface{}]*item, lc.size)
	lc.lruList = list.New()
}

// Count return count of items, query and hit
func (lc *LRUCache) Count() (cntItems, cntQuery, cntHit uint64) {
	lc.m.RLock()
	defer lc.m.RUnlock()

	return uint64(lc.lruList.Len()), lc.cntQuery, lc.cntHit
}
