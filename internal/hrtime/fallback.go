package hrtime

import "time"

var origin = time.Now()

func fallback() int64 { return int64(time.Since(origin)) }
