package identitystore

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

// Put and Delete describe one transactional durable-store change.
type Put struct {
	Bucket string
	Key    []byte
	Value  []byte
}

type Delete struct {
	Bucket string
	Key    []byte
}

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

func insert(bucket string, key []byte, value []byte) error {
	return insertMany([]Put{{Bucket: bucket, Key: key, Value: value}})
}

// insertMany creates every record at one revision, refusing to overwrite an
// existing record. A concurrent commit is retried against the new revision.
func insertMany(values []Put) error {
	for attempt := 0; attempt < retries; attempt++ {
		revision, err := currentRevision()
		if err != nil {
			return err
		}
		for _, value := range values {
			_, found, err := getValue(value.Bucket, value.Key)
			if err != nil {
				return err
			}
			if found {
				return errors.New("identity record already exists")
			}
		}
		changes := make([]storageTypes.Change, 0, len(values))
		for _, value := range values {
			changes = append(changes, putChange(value.Bucket, value.Key, value.Value))
		}
		committed, err := commitChanges(revision, changes)
		if err != nil {
			return err
		}
		if committed {
			return nil
		}
	}
	return errors.New("identity state changed too frequently")
}

// replace overwrites an existing record and fails when it disappeared.
func replace(bucket string, key []byte, value []byte) error {
	for attempt := 0; attempt < retries; attempt++ {
		revision, err := currentRevision()
		if err != nil {
			return err
		}
		_, found, err := getValue(bucket, key)
		if err != nil {
			return err
		}
		if !found {
			return errors.New("identity record not found")
		}
		committed, err := commitChanges(revision, []storageTypes.Change{putChange(bucket, key, value)})
		if err != nil {
			return err
		}
		if committed {
			return nil
		}
	}
	return errors.New("identity state changed too frequently")
}

// takeValue reads and deletes one record atomically. The caller retries when
// the record state changed between the read and the commit.
func takeValue(bucket string, key []byte) ([]byte, bool, error) {
	for attempt := 0; attempt < retries; attempt++ {
		revision, err := currentRevision()
		if err != nil {
			return nil, false, err
		}
		value, found, err := getValue(bucket, key)
		if err != nil {
			return nil, false, err
		}
		if !found {
			return nil, false, nil
		}
		committed, err := commitChanges(revision, []storageTypes.Change{deleteChange(bucket, key)})
		if err != nil {
			return nil, false, err
		}
		if committed {
			return value, true, nil
		}
	}
	return nil, false, errors.New("identity state changed too frequently")
}

func removeValue(bucket string, key []byte) (bool, error) {
	_, found, err := takeValue(bucket, key)
	if err != nil {
		return false, err
	}
	return found, nil
}

// removeMany deletes every record in one commit and reports whether any of
// them existed.
func removeMany(values []Delete) (bool, error) {
	for attempt := 0; attempt < retries; attempt++ {
		revision, err := currentRevision()
		if err != nil {
			return false, err
		}
		found := false
		for _, value := range values {
			_, exists, err := getValue(value.Bucket, value.Key)
			if err != nil {
				return false, err
			}
			found = found || exists
		}
		if !found {
			return false, nil
		}
		changes := make([]storageTypes.Change, 0, len(values))
		for _, value := range values {
			changes = append(changes, deleteChange(value.Bucket, value.Key))
		}
		committed, err := commitChanges(revision, changes)
		if err != nil {
			return false, err
		}
		if committed {
			return true, nil
		}
	}
	return false, errors.New("identity state changed too frequently")
}

// scanAll reads a whole bucket at a single revision, retrying when the store
// revision moves between pages.
func scanAll(bucket string) ([]storageTypes.Entry, error) {
	for attempt := 0; attempt < retries; attempt++ {
		expected, err := currentRevision()
		if err != nil {
			return nil, err
		}
		entries := make([]storageTypes.Entry, 0)
		var after witTypes.Option[[]byte] = witTypes.None[[]byte]()
		changed := false
		for {
			pageResult := ohrats_rc_storage_durable_store.Scan(bucket, []byte{}, after, pageSize)
			if pageResult.IsErr() {
				return nil, errors.New(pageResult.Err())
			}
			page := pageResult.Ok()
			if page.Revision != expected {
				changed = true
				break
			}
			entries = append(entries, page.Entries...)
			if page.NextKey.IsNone() {
				break
			}
			after = page.NextKey
		}
		if !changed {
			return entries, nil
		}
	}
	return nil, errors.New("identity state changed too frequently")
}

func currentRevision() (uint64, error) {
	result := ohrats_rc_storage_durable_store.Revision()
	if result.IsErr() {
		return 0, errors.New(result.Err())
	}
	return result.Ok(), nil
}

func commitChanges(revision uint64, changes []storageTypes.Change) (bool, error) {
	result := ohrats_rc_storage_durable_store.Commit(revision, changes)
	if result.IsErr() {
		failure := result.Err()
		if failure.Tag() == storageTypes.CommitError_Conflict {
			return false, nil
		}
		return false, errors.New(failure.Failure())
	}
	return true, nil
}

func putChange(bucket string, key []byte, value []byte) storageTypes.Change {
	return storageTypes.MakeChangePut(storageTypes.Entry{Bucket: bucket, Key: key, Value: value})
}

func deleteChange(bucket string, key []byte) storageTypes.Change {
	return storageTypes.MakeChangeDelete(storageTypes.Deletion{Bucket: bucket, Key: key})
}
