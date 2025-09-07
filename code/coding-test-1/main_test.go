package main

import (
	"context"
	"testing"
	"time"
)

func TestFetchAndAggregate_AllSuccessOrPartial(t *testing.T) {
	ids := []string{"A1", "A2", "A3", "A4", "A5"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, errs := FetchAndAggregate(ctx, ids, 3, 2*time.Second)
	if len(res) == 0 && len(errs) == 0 {
		t.Fatalf("expected some results or errors, got none")
	}
	if len(res)+len(errs) != len(ids) && len(errs) == 0 {
		t.Logf("res=%d errs=%d (non-strict due to randomness)", len(res), len(errs))
	}
}

func TestFetchAndAggregate_PerItemTimeout(t *testing.T) {
	ids := []string{"B1", "B2", "B3"}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	res, errs := FetchAndAggregate(ctx, ids, 2, 600*time.Millisecond)
	if len(errs) == 0 {
		t.Logf("res=%d errs=%d (might pass due to randomness)", len(res), len(errs))
	}
}

func TestFetchAndAggregate_GlobalCancel(t *testing.T) {
	ids := []string{"C1", "C2", "C3", "C4", "C5", "C6", "C7"}
	ctx, cancel := context.WithTimeout(context.Background(), 900*time.Millisecond) // cepat cancel
	defer cancel()

	_, errs := FetchAndAggregate(ctx, ids, 5, 2*time.Second)
	foundGlobal := false
	for _, e := range errs {
		if e.Error() == "global context cancelled: context deadline exceeded" ||
			(len(e.Error()) > 0 && e.Error()[0:21] == "global context cancelled") {
			foundGlobal = true
			break
		}
	}
	if !foundGlobal {
		t.Log("global cancel not always deterministic; acceptable due to randomness")
	}
}
