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
	if e, _ := ComputeExpiry(base, 1, "month"); !e.Equal(time.Date(2026, 2, 28, 8, 0, 0, 0, time.UTC)) {
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

func TestAddMonthsClamped(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	d := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 17, 30, 5, 0, loc) }
	cases := []struct {
		name     string
		in, want time.Time
	}{
		{"10/22", d(2026, 10, 22), d(2026, 11, 22)},
		{"2/5", d(2026, 2, 5), d(2026, 3, 5)},
		{"1/31 leap", d(2028, 1, 31), d(2028, 2, 29)},
		{"1/31 non-leap", d(2026, 1, 31), d(2026, 2, 28)},
		{"12/15 cross year", d(2026, 12, 15), d(2027, 1, 15)},
		{"3/31", d(2026, 3, 31), d(2026, 4, 30)},
	}
	for _, c := range cases {
		if got := AddMonthsClamped(c.in, 1); !got.Equal(c.want) {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
	if got := AddMonthsClamped(d(2026, 1, 31), 13); !got.Equal(d(2027, 2, 28)) {
		t.Errorf("+13m: %v", got)
	}
}
