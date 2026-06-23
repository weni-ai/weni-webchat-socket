package starters

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCacheKey_UsesProductPathWhenPresent(t *testing.T) {
	assert.Equal(t, "store:/en/ipad/p", CacheKey("store", "/en/ipad/p", "ipad"))
}

func TestCacheKey_FallsBackToLinkText(t *testing.T) {
	assert.Equal(t, "store:ipad", CacheKey("store", "", "ipad"))
}
