# 项目名称

Exchange Cache提供基于内存的缓存功能，主要用于缓存一些使用频率较高，且不会更新或者对更新时效性要求不高的数据。

## 快速开始

### 使用LRUCache

ExCache的LRU Cache在传统的LRU Cache基础上增加了使用次数和有效期的限制，可以在初始化时设置。

**创建一个LRUCache**
> `func NewLRUCache(size int, uctl int, ttl time.Duration) *LRUCache`

- size: Cache中能存放的最大记录量
- uctl(Used-Count-To-Live): 每条记录最多可被使用的次数，如果为0表示不限制使用次数
- ttl(Time-To-Live): 每条记录最长的有效期，如果为0表示不限制有效期

**写记录入LRUCache**
> `func (lc *LRUCache) Set(key, val interface{}) {`

**从LRUCache中读记录**
> `func (lc *LRUCache) Get(key interface{}) (interface{}, bool) {`

**清空LRUCache**
>`func (lc *LRUCache) Purge() {`

**统计LRUCache的计数**
>   `func (lc *LRUCache) Count() (cntItems, cntQuery, cntHit uint64) {`

- cntItems: 当前LRUCache中的记录数，包括已过期的
- cntQuery: 当前LRUCache的请求次数
- cntHit: LRUCache的命中次数

**示例**

```go
// 设置一个最大容量为2000条记录，每条最多可以使用10次，且仅生效一分钟的LRUcache
mycache := excache.NewLRUCache(2000, 10, 60*time.Second)
// 将记录写入缓存
mycache.Set("someKey", "someValue")
// 从缓存中获取值
if v, ok := mycache.Get("someKey"); ok {
    print(v.(string))
}
```

### 使用PersistentLRUCache

PersistentLRUCache在ExCache LRUCache基础上增加了持久化到本地文件的功能，对于永远不变的数据，本地文件可提供远超内存容量的缓存量；对于会变化的数据，本地文件中的缓存记录可作为容灾降级数据。

**创建一个PersistentLRUCache**
> `func NewPersistentLRUCache(db string, bucketSize int, size int, uctl int, ttl time.Duration) (p *PersistentLRUCache, err error)`

- db: 数据文件的地址
- bucketSize: 桶的容量
- size: 内存Cache的容量
- uctl：内存中每条记录最多可被使用的次数，如果为0表示不限制使用次数
- ttl: 内存中每条记录最长的有效期，如果为0表示不限制有效期

**设置批量持久化的数量**
> `func (p *PersistentLRUCache) SetBatchSize(n int)`

若不设置，默认为1，即没有批量操作，每一条写入都会执行持久化操作。一条数据的持久化操作每次约10ms左右，根据目前线上数据的经验，1000条1k数据的持久化约600ms。对于吞吐量较大的场景可以考虑使用批量。

**设置两次持久化间的间隔时间**
> `func (p *PersistentLRUCache) SetBatchGapTime(t time.Duration)`

若不设置，默认为10秒。当使用批量持久化时，为避免长时间不持久化的风险，可设置两次持久化间的间隔时间，即上一次持久化结束到下一次持久化开始的时间间隔。

**设置异步持久化的Channel大小**
> `func (p *PersistentLRUCache) SetChannelSize(n int) error`

PersistentLRUCache的持久化是使用一个单独的Goroutine执行操作，业务侧调用时会将数据通过Channel传到的持久化协程，Channel的大小默认是10000，业务方可根据自己的业务情况调整。*`调整时需保证Channel中没有未消费的数据，否则会改动无效并返回excache.ErrWriteChannelNotEmpty的错误。`*

**异步写入**
> `func (p *PersistentLRUCache) Set(key, val interface{}, errCh chan<- error)`

key和val分别是需要写入的key和value。Set方法会同时设置内存Cache并持久化到文件中，写文件是异步的，调用返回时表示数据已经写入持久化的Channel待持久化Goroutine处理。errCh是业务侧创建并传入的channel，持久化Goroutine会在处理失败的时候通过errCh将错误返回。当使用批量化写入时，只有恰好触发了持久化（待持久化的数量刚好满足条件）的写入会有error返回，其它的均会返回nil。errCh可以设置为nil，表示忽略持久化的错误。

**同步写入**
> `func (p *PersistentLRUCache) SyncSet(key, val interface{}) error`

SyncSet是Set的同步方法，会在内存Cache设置完，并且持久化Goroutine也处理完数据的时候才返回。在使用批量化写入的时候，同样是只有触发持久化的写入才会返回错误，其它均返回nil。

**获取**
> `func (p *PersistentLRUCache) Get(key interface{}, allocFunc func() interface{}) (val interface{}, ok bool, err error)`

Get方法传入Key，同时需要传入一个能够获取与Value相同格式的内存指针的函数，用于将文件中的数据进行反序列化。
val是返回的数据，ok表示数据是否存在，当ok为false的时候，val也是nil。err会返回获取过程中的错误，一般只有excache.ErrItemExpired表示数据在内存中不存在，是在备份文件中找到的。

**清空备份文件**
> `func (p *PersistentLRUCache) PurgeBackup() error`

**关闭Cache**
> `func (p *PersistentLRUCache) Close()`

Close会关闭备份文件的句柄，保证数据不会因为写到一半被损坏。

## 测试

如何执行自动化测试？

## 如何贡献

提交PR -> fork分支 -> 开发代码 -> 提CR -> 管理员merge

## 讨论

QQ讨论群：XXXX
