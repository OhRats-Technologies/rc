package storagefixture

import (
	"errors"
	"fmt"

	"wit_component/ohrats_rc_storage_durable_store"
	"wit_component/ohrats_rc_storage_types"
)

// Invoke dispatches one storage-fixture command.
func Invoke(command string, args []string) (uint32, error) {
	switch command {
	case "kv-set":
		return set(args)
	case "kv-get":
		return get(args)
	case "kv-list":
		return list(args)
	case "kv-delete":
		return deleteKey(args)
	case "kv-conflict":
		return conflict(args)
	default:
		return 0, fmt.Errorf("unsupported command %s", quoted(command))
	}
}

func set(args []string) (uint32, error) {
	if len(args) != 3 {
		return 0, errors.New("usage: rc kv-set <bucket> <key> <value>")
	}
	revision, err := currentRevision()
	if err != nil {
		return 0, err
	}
	change := ohrats_rc_storage_types.MakeChangePut(ohrats_rc_storage_types.Entry{
		Bucket: args[0],
		Key:    []byte(args[1]),
		Value:  []byte(args[2]),
	})
	if err := commit(revision, []ohrats_rc_storage_types.Change{change}); err != nil {
		return 0, err
	}
	return 0, nil
}

func get(args []string) (uint32, error) {
	if len(args) != 2 {
		return 0, errors.New("usage: rc kv-get <bucket> <key>")
	}
	result := ohrats_rc_storage_durable_store.Get(args[0], []byte(args[1]))
	if result.IsErr() {
		return 0, errors.New(result.Err())
	}
	option := result.Ok()
	if !option.IsSome() {
		return 0, fmt.Errorf("key %s is not set", quoted(args[1]))
	}
	fmt.Println(lossyUTF8(option.Some()))
	return 0, nil
}

func list(args []string) (uint32, error) {
	if len(args) != 1 && len(args) != 2 {
		return 0, errors.New("usage: rc kv-list <bucket> [prefix]")
	}
	prefix := ""
	if len(args) == 2 {
		prefix = args[1]
	}
	result := ohrats_rc_storage_durable_store.Scan(
		args[0],
		[]byte(prefix),
		noneKey(),
		100,
	)
	if result.IsErr() {
		return 0, errors.New(result.Err())
	}
	for _, entry := range result.Ok().Entries {
		fmt.Printf("%s=%s\n", lossyUTF8(entry.Key), lossyUTF8(entry.Value))
	}
	return 0, nil
}

func deleteKey(args []string) (uint32, error) {
	if len(args) != 2 {
		return 0, errors.New("usage: rc kv-delete <bucket> <key>")
	}
	revision, err := currentRevision()
	if err != nil {
		return 0, err
	}
	change := ohrats_rc_storage_types.MakeChangeDelete(ohrats_rc_storage_types.Deletion{
		Bucket: args[0],
		Key:    []byte(args[1]),
	})
	if err := commit(revision, []ohrats_rc_storage_types.Change{change}); err != nil {
		return 0, err
	}
	return 0, nil
}

func conflict(args []string) (uint32, error) {
	if len(args) != 0 {
		return 0, errors.New("usage: rc kv-conflict")
	}
	revision, err := currentRevision()
	if err != nil {
		return 0, err
	}
	first := ohrats_rc_storage_types.MakeChangePut(ohrats_rc_storage_types.Entry{
		Bucket: "conflict",
		Key:    []byte("first"),
		Value:  []byte("one"),
	})
	if err := commit(revision, []ohrats_rc_storage_types.Change{first}); err != nil {
		return 0, err
	}
	second := ohrats_rc_storage_types.MakeChangePut(ohrats_rc_storage_types.Entry{
		Bucket: "conflict",
		Key:    []byte("second"),
		Value:  []byte("two"),
	})
	result := ohrats_rc_storage_durable_store.Commit(
		revision,
		[]ohrats_rc_storage_types.Change{second},
	)
	if result.IsErr() {
		failure := result.Err()
		if failure.Tag() == ohrats_rc_storage_types.CommitError_Conflict {
			fmt.Printf("conflict at revision %d\n", failure.Conflict())
			return 0, nil
		}
		return 0, commitFailure(failure)
	}
	return 0, errors.New("stale durable-store commit unexpectedly succeeded")
}

func currentRevision() (uint64, error) {
	result := ohrats_rc_storage_durable_store.Revision()
	if result.IsErr() {
		return 0, errors.New(result.Err())
	}
	return result.Ok(), nil
}

func commit(revision uint64, changes []ohrats_rc_storage_types.Change) error {
	result := ohrats_rc_storage_durable_store.Commit(revision, changes)
	if result.IsErr() {
		return commitFailure(result.Err())
	}
	return nil
}

func commitFailure(failure ohrats_rc_storage_types.CommitError) error {
	if failure.Tag() == ohrats_rc_storage_types.CommitError_Conflict {
		return fmt.Errorf("data changed at revision %d", failure.Conflict())
	}
	return errors.New(failure.Failure())
}
