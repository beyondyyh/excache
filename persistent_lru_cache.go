package excache

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/boltdb/bolt"
	"github.com/vmihailenco/msgpack"
)

var (
	// ErrItemExpired if item is got from local storage
	ErrItemExpired = errors.New("item was expired")
	// ErrWriteChannelNotEmpty if write channel not empty
	ErrWriteChannelNotEmpty = errors.New("write channel not empty")

	_DefaultWriteChannelSize = 10000
	_DefaultCacheBucket      = "excache"
	_DefaultBatchGapTime     = 10 * time.Second
)

// DefaultBatchSize 批量落盘数量, 1标识不批量，默认开启批量，为100
// 如果不批量写磁盘，在高并发时会对I/O造成非常大的压力，可能导致程序夯住不响应的情况。
var DefaultBatchSize = 100 // no batch

// PLRUStat statistic numbers
type PLRUStat struct {
	LRUItem  uint64
	LRUQuery uint64
	LRUHit   uint64
	DBStat   bolt.Stats
}

// PLRUConfig 持久化缓存的创建参数
type PLRUConfig struct {
	DBPath           string        // 持久化缓存的文件路径
	BucketSize       int           // 持久化的单桶记录数，实际持久化最大数量为两个桶
	MemorySize       int           // 内存LRU缓存的记录数
	UseCountToLive   int           // 单条LRU缓存的有效访问次数，0为无限制
	TimeToLive       time.Duration // 单条LRU缓存的有效访问时间
	BatchSize        int           // 批量持久化记录数，小于等于1为每次持久化一条，即不批量持久化
	BatchGapTime     time.Duration // 持久化未达到批量数量时强制持久化的间隔时间
	WriteChannelSize int           // 缓存的信道数量，0为使用默认值10,000个信道
}

type asyncSet struct {
	key interface{}
	val interface{}
	err chan<- error
}

type serializable interface {
	Load([]byte)
	Dump() []byte
}

// PersistentLRUCache LRUCache with local storage
type PersistentLRUCache struct {
	*LRUCache
	db         *bolt.DB
	bucketSize int

	batchSize    int
	batchGapTime time.Duration
	currBuckID   uint8
	setQueue     chan asyncSet
	closeCh      chan struct{}
}

// NewPersistentLRUCache return new persistent lru cache
func NewPersistentLRUCache(db string, bucketSize int, size int, uctl int, ttl time.Duration) (
	p *PersistentLRUCache, err error) {
	conf := &PLRUConfig{
		DBPath:         db,
		BucketSize:     bucketSize,
		MemorySize:     size,
		UseCountToLive: uctl,
		TimeToLive:     ttl,
	}
	return NewPLRUCache(conf)
}

// NewPLRUCache create new persistent lru cache
func NewPLRUCache(conf *PLRUConfig) (p *PersistentLRUCache, err error) {
	if conf.BucketSize <= 0 {
		panic("bucketSize should greater than zero")
	}
	bgTime := conf.BatchGapTime
	if int64(bgTime) == 0 {
		bgTime = _DefaultBatchGapTime
	}
	wcSize := conf.WriteChannelSize
	if wcSize == 0 {
		wcSize = _DefaultWriteChannelSize
	}
	p = &PersistentLRUCache{
		LRUCache:     NewLRUCache(conf.MemorySize, conf.UseCountToLive, conf.TimeToLive),
		bucketSize:   conf.BucketSize,
		batchSize:    conf.BatchSize,
		batchGapTime: bgTime,
		currBuckID:   0,
		setQueue:     make(chan asyncSet, wcSize),
		closeCh:      make(chan struct{}),
	}
	p.db, err = bolt.Open(conf.DBPath, os.ModePerm, &bolt.Options{})
	if err == nil {
		go p.backupLoop()
	}
	return
}

// SetBatchSize modify batch size
func (p *PersistentLRUCache) SetBatchSize(n int) {
	p.batchSize = n
}

// SetBatchGapTime set gap time of batch
func (p *PersistentLRUCache) SetBatchGapTime(t time.Duration) {
	p.batchGapTime = t
}

// SetChannelSize only works when channel is empty
func (p *PersistentLRUCache) SetChannelSize(n int) error {
	if len(p.setQueue) > 0 {
		return ErrWriteChannelNotEmpty
	}
	close(p.setQueue)
	p.setQueue = make(chan asyncSet, n)
	return nil
}

// Stat statistics
func (p *PersistentLRUCache) Stat() *PLRUStat {
	ret := &PLRUStat{}
	ret.LRUItem, ret.LRUQuery, ret.LRUHit = p.LRUCache.Count()
	ret.DBStat = p.db.Stats()

	return ret
}

// Set value with key, and write to local storage.
// Set is non-block api for write local storage
func (p *PersistentLRUCache) Set(key, val interface{}, errCh chan<- error) {
	p.LRUCache.Set(key, val)

	p.setQueue <- asyncSet{
		key: key,
		val: val,
		err: errCh,
	}
}

// SyncSet is same as Set() except block and return error
func (p *PersistentLRUCache) SyncSet(key, val interface{}) error {
	errCh := make(chan error)
	p.Set(key, val, errCh)
	return <-errCh
}

// Get try get from memory or disk
func (p *PersistentLRUCache) Get(key interface{}, allocFunc func() interface{}) (
	val interface{}, ok bool, err error) {
	val, ok = p.LRUCache.Get(key)
	if ok {
		return val, ok, nil
	}
	err = p.db.View(func(tx *bolt.Tx) error {
		bkey, err := msgpack.Marshal(key)
		if err != nil {
			return fmt.Errorf("Pack key error: %s", err.Error())
		}
		var bucketSeek = func(bn []byte) []byte {
			bucket := tx.Bucket(bn)
			if bucket == nil {
				return nil
			}
			bval := bucket.Get(bkey)
			if bval == nil {
				return nil
			}
			return bval
		}
		bval := bucketSeek(p.firstBucket())
		if bval == nil {
			bval = bucketSeek(p.sencondBucket())
		}
		if bval == nil {
			return nil
		}
		val = allocFunc()
		if serialier, ok := val.(serializable); ok {
			serialier.Load(bval)
		} else {
			err = msgpack.Unmarshal(bval, val)
			if err != nil {
				// fmt.Printf("bval: %s, val: %+v, err: %+v\n", string(bval), val, err)
				return err
			}
		}
		ok = true
		return ErrItemExpired
	})
	return val, ok, err
}

// PurgeBackup purge backup
func (p *PersistentLRUCache) PurgeBackup() error {
	err := p.db.Update(func(tx *bolt.Tx) error {
		err := tx.DeleteBucket(p.firstBucket())
		if err != nil {
			return err
		}
		return tx.DeleteBucket(p.sencondBucket())
	})
	if err == bolt.ErrBucketNotFound {
		return nil
	}
	return err
}

// Close close persistent db
func (p *PersistentLRUCache) Close() {
	p.closeCh <- struct{}{}
}

func (p *PersistentLRUCache) firstBucket() []byte {
	return []byte(fmt.Sprintf("%s_%d", _DefaultCacheBucket, p.currBuckID))
}

func (p *PersistentLRUCache) sencondBucket() []byte {
	return []byte(fmt.Sprintf("%s_%d", _DefaultCacheBucket, 1-p.currBuckID))
}

func (p *PersistentLRUCache) backupLoop() {
	batch := make([]asyncSet, 0)
	timer := time.NewTimer(p.batchGapTime)
	for {
		select {
		case <-p.closeCh:
			timer.Stop()
			p.db.Close()
			break
		case <-timer.C:
			p.writeBatchToDB(batch) // ignore error, cause last error channel may be closed
			batch = make([]asyncSet, 0)
		case msg := <-p.setQueue:
			batch = append(batch, msg)
			if len(batch) < p.batchSize {
				if msg.err != nil {
					msg.err <- nil
				}
				continue
			}
			err := p.writeBatchToDB(batch)
			batch = make([]asyncSet, 0)
			if msg.err != nil {
				msg.err <- err
			}
		}
		timer.Reset(p.batchGapTime)
	}
}

func (p *PersistentLRUCache) writeBatchToDB(batch []asyncSet) error {
	if len(batch) == 0 {
		return nil
	}
	err := p.db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(p.firstBucket())
		if err != nil {
			return fmt.Errorf("Create bucket: %s", err.Error())
		}
		for _, record := range batch {
			bkey, err := msgpack.Marshal(record.key)
			if err != nil {
				return fmt.Errorf("Pack Key error: %s", err.Error())
			}
			var bval []byte
			if serializer, ok := record.val.(serializable); ok {
				bval = serializer.Dump()
			} else {
				bval, err = msgpack.Marshal(record.val)
				if err != nil {
					return fmt.Errorf("Pack value error: %s", err.Error())
				}
			}
			if err := bucket.Put(bkey, bval); err != nil {
				return err
			}
		}
		if bucket.Stats().KeyN >= p.bucketSize {
			// Ignore rotate failed
			err := tx.DeleteBucket(p.sencondBucket())
			if err == nil {
				p.currBuckID = 1 - p.currBuckID
			}
		}
		return nil
	})
	return err
}
