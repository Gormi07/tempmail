package model

import (
	"testing"
	"time"
)

func TestComputeExpiry(t *testing.T) {
	base := time.Date(2026, 1, 31, 8, 0, 0, 0, time.UTC)
	if e, err := ComputeExpiry(base, 0, "day"); err != nil || e != nil {
		t.Fatalf("0 should be permanent, got %v %v", e, err)
	}
	if e, _ := ComputeExpiry(base, 7, "day"); !e.Equal(base.AddDate(0, 0, 7)) {
		t.Fatalf("7 days: %v", e)
	}
	if e, _ := ComputeExpiry(base, 1, "month"); !e.Equal(base.AddDate(0, 1, 0)) {
		t.Fatalf("1 month: %v", e)
	}
	if _, err := ComputeExpiry(base, 1, "year"); err == nil {
		t.Fatal("expected error for invalid unit")
	}
}

func TestIsExpired(t *testing.T) {
	n := time.Now()
	p, f := n.Add(-time.Minute), n.Add(time.Minute)
	if (&Account{}).IsExpired(n) {
		t.Error("nil expiry must be permanent")
	}
	if !(&Account{ExpiresAt: &p}).IsExpired(n) || (&Account{ExpiresAt: &f}).IsExpired(n) {
		t.Error("expiry comparison wrong")
	}
}
