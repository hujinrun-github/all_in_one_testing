package api

import (
	"fmt"
	"sync/atomic"
	"time"
)

var resourceIDSequence atomic.Uint64

func newResourceID(prefix string) string {
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UTC().UnixNano(), resourceIDSequence.Add(1))
}
