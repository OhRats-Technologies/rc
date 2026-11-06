package credentialstore

import (
	"math"
	"strconv"

	storageTypes "wit_component/ohrats_rc_storage_types"
)

// clockSkew returns the distance between two seconds without overflowing on
// extreme timestamp values.
func clockSkew(now int64, timestamp int64) uint64 {
	if now >= timestamp {
		return uint64(now - timestamp)
	}
	return uint64(timestamp - now)
}

func nonceExpiryValue(nowMs uint64) uint64 {
	if nowMs > math.MaxUint64-120000 {
		return math.MaxUint64
	}
	return nowMs + 120000
}

// nonceExpiry reads the decimal expiry stored with a nonce. A value that is
// not a decimal expiry is not treated as a recorded nonce.
func nonceExpiry(value []byte) (uint64, bool) {
	expires, err := strconv.ParseUint(string(value), 10, 64)
	if err != nil {
		return 0, false
	}
	return expires, true
}

func expiredNonceDeletions(nowMs uint64, current string) ([]storageTypes.Change, error) {
	entries, err := scanLimit(noncesBucket, 128)
	if err != nil {
		return nil, err
	}
	changes := make([]storageTypes.Change, 0, len(entries))
	for _, entry := range entries {
		if string(entry.Key) == current {
			continue
		}
		expires, ok := nonceExpiry(entry.Value)
		if !ok || expires > nowMs {
			continue
		}
		changes = append(changes, deleteChange(noncesBucket, entry.Key))
	}
	return changes, nil
}
