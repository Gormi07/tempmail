package handler

import (
	"testing"

	"github.com/google/uuid"
)

func TestParseBatchDeleteIDs(t *testing.T) {
	if _, err := ParseBatchDeleteIDs(nil); err == nil || err.Error() != "请至少选择一个账户" {
		t.Fatalf("empty: %v", err)
	}
	if _, err := ParseBatchDeleteIDs([]string{"bad"}); err == nil || err.Error() != "账户 ID 无效" {
		t.Fatalf("bad id: %v", err)
	}
	big := make([]string, MaxBatchDelete+1)
	for i := range big {
		big[i] = uuid.NewString()
	}
	if _, err := ParseBatchDeleteIDs(big); err == nil {
		t.Fatal("expected limit error")
	}
	id := uuid.NewString()
	ids, err := ParseBatchDeleteIDs([]string{id, " " + id + " "})
	if err != nil || len(ids) != 1 {
		t.Fatalf("dedupe: %v %v", ids, err)
	}
}
