package executionruntime

import "testing"

func TestDuplicateExecutionIDIsNeverReclaimed(t *testing.T) {
	if err := claimExecutionID("execution-duplicate"); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	err := claimExecutionID("execution-duplicate")
	if err == nil || err.Error() != "execution id was already claimed" {
		t.Fatalf("duplicate claim = %v, want duplicate error", err)
	}
}

func TestEmptyExecutionIDIsRejectedBeforeNativeSpawn(t *testing.T) {
	err := claimExecutionID("")
	if err == nil || err.Error() != "execution id is empty" {
		t.Fatalf("empty claim = %v, want empty id error", err)
	}
}

func TestActiveCountsAreLifetimeSpecificAndSaturating(t *testing.T) {
	base := claims.active

	if started(KindManaged) != [3]int{base[0], base[1] + 1, base[2]} {
		t.Fatal("managed start did not increment the managed count")
	}
	if started(KindAttached) != [3]int{base[0] + 1, base[1] + 1, base[2]} {
		t.Fatal("attached start did not increment the attached count")
	}
	if finished(KindManaged) != [3]int{base[0] + 1, base[1], base[2]} {
		t.Fatal("managed finish did not decrement the managed count")
	}
	if finished(KindManaged) != claims.active {
		t.Fatal("managed finish underflowed the active count")
	}
}
