package iplookup

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCacheDuration(t *testing.T) {

	// Single Get
	ipl := &IPLookup{
		cacheLookup: make(map[string]*cacheEntry),
		maxEntries:  3,
		maxDuration: 10 * time.Second,
	}

	ipl.addCache("e1", "v1")

	ipl.Lock()
	e1, found := ipl.cacheLookup["e1"]
	ipl.Unlock()

	assert.NotNil(t, e1)
	assert.True(t, found)
	assert.Equal(t, "e1", e1.name)
	assert.Equal(t, "v1", e1.value)

	// Add 3 more
	ipl.addCache("e2", "v2")
	ipl.addCache("e3", "v3")
	ipl.addCache("e4", "v4")

	// Should be missing now
	ipl.Lock()
	e1, found = ipl.cacheLookup["e1"]
	ipl.Unlock()

	assert.Nil(t, e1)
	assert.False(t, found)

	// Clean it to 1 entry
	ipl.Lock()
	ipl.maxEntries = 1
	ipl.cleanCache()
	ipl.Unlock()

	_, found = ipl.cacheLookup["e1"]
	assert.False(t, found)
	_, found = ipl.cacheLookup["e2"]
	assert.False(t, found)
	_, found = ipl.cacheLookup["e3"]
	assert.False(t, found)
	e4, found := ipl.cacheLookup["e4"]
	assert.NotNil(t, e4)
	assert.True(t, found)
	assert.Equal(t, "e4", e4.name)
	assert.Equal(t, "v4", e4.value)

	// Force e4 to expire
	ipl.Lock()
	e4.expires = time.Now().Add(-1 * time.Second)
	ipl.cleanCache()
	ipl.Unlock()

	// Cache should be empty
	assert.Nil(t, ipl.head)
	assert.Nil(t, ipl.tail)
	assert.Equal(t, 0, ipl.size)
	assert.Equal(t, 0, len(ipl.cacheLookup))
}
