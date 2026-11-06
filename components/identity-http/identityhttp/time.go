package identityhttp

import "time"

func nowMs() uint64 {
	value := time.Now().UnixMilli()
	if value < 0 {
		return 0
	}
	return uint64(value)
}

func saturatingAdd(value uint64, amount uint64) uint64 {
	if value > ^uint64(0)-amount {
		return ^uint64(0)
	}
	return value + amount
}
