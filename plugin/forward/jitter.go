package forward

import (
	"math/rand"
	"time"
)

// jitter adds up to 25% jitter to the expire duration.
func jitter(expire time.Duration) time.Duration {
	exp := int64(expire / 4)
	return expire + time.Duration(rand.Int63n(exp))
}
