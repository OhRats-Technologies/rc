package credentialstore

import (
	"errors"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_storage_durable_store"
	storageTypes "wit_component/ohrats_rc_storage_types"
)

const (
	retries  = 8
	pageSize = 1000
)

func getValue(bucket string, key []byte) ([]byte, bool, error) {
	result := ohrats_rc_storage_durable_store.Get(bucket, key)
	if result.IsErr() {
		return nil, false, errors.New(result.Err())
	}
	option := result.Ok()
	if option.IsNone() {
		return nil, false, nil
	}
	return option.Some(), true, nil
}

func revision() (uint64, error) {
	result := ohrats_rc_storage_durable_store.Revision()
	if result.IsErr() {
		return 0, errors.New(result.Err())
	}
	return result.Ok(), nil
}

// putValue commits one entry, retrying when another writer moves the store
// revision between the read and the commit.
func putValue(bucket string, key []byte, value []byte) error {
	for attempt := 0; attempt < retries; attempt++ {
		expected, err := revision()
		if err != nil {
			return err
		}
		committed, err := commitOnce(expected, []storageTypes.Change{putChange(bucket, key, value)})
		if err != nil {
			return err
		}
		if committed {
			return nil
		}
	}
	return errors.New("API credential state changed too frequently")
}

// commitOnce reports whether the commit succeeded. A conflict is not an error.
func commitOnce(expected uint64, changes []storageTypes.Change) (bool, error) {
	result := ohrats_rc_storage_durable_store.Commit(expected, changes)
	if !result.IsErr() {
		return true, nil
	}
	failure := result.Err()
	if failure.Tag() == storageTypes.CommitError_Conflict {
		return false, nil
	}
	return false, errors.New(failure.Failure())
}

// scanBucket reads a whole bucket by paging through it.
func scanBucket(bucket string) ([]storageTypes.Entry, error) {
	entries := make([]storageTypes.Entry, 0)
	var after witTypes.Option[[]byte] = witTypes.None[[]byte]()
	for {
		pageResult := ohrats_rc_storage_durable_store.Scan(bucket, []byte{}, after, pageSize)
		if pageResult.IsErr() {
			return nil, errors.New(pageResult.Err())
		}
		page := pageResult.Ok()
		entries = append(entries, page.Entries...)
		if page.NextKey.IsNone() {
			return entries, nil
		}
		after = page.NextKey
	}
}

func scanLimit(bucket string, limit uint32) ([]storageTypes.Entry, error) {
	pageResult := ohrats_rc_storage_durable_store.Scan(bucket, []byte{}, witTypes.None[[]byte](), limit)
	if pageResult.IsErr() {
		return nil, errors.New(pageResult.Err())
	}
	return pageResult.Ok().Entries, nil
}

func putChange(bucket string, key []byte, value []byte) storageTypes.Change {
	return storageTypes.MakeChangePut(storageTypes.Entry{Bucket: bucket, Key: key, Value: value})
}

func deleteChange(bucket string, key []byte) storageTypes.Change {
	return storageTypes.MakeChangeDelete(storageTypes.Deletion{Bucket: bucket, Key: key})
}
