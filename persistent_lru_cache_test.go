package excache

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var testdb = "./test.db"

// run: go test -v -run TestMain
func TestMain(m *testing.M) {
	delTestFile()
	defer delTestFile()

	m.Run()
}

func delTestFile() {
	os.RemoveAll(testdb)
}

// run: go test -v -run TestPersistentLRUCache
func TestPersistentLRUCache(t *testing.T) {
	cache, err := NewPersistentLRUCache(testdb, 3, 3, 1, 1*time.Second)
	assert.NoError(t, err)
	cache.SetBatchSize(1)
	cache.SetBatchGapTime(time.Millisecond)
	cache.SetChannelSize(1)
	err = cache.SyncSet("key", "val")
	assert.NoError(t, err)
	// Get Once
	val, ok, err := cache.Get("key", nil)
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "val", val)
	// Get Twice
	val, ok, err = cache.Get("key", func() interface{} {
		a := ""
		return &a
	})
	assert.Error(t, err)
	assert.True(t, ok)
	assert.Equal(t, "val", *(val.(*string)))
	// PurgeBackup
	assert.Nil(t, cache.PurgeBackup())
}
