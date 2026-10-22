package handler

import "testing"

func TestCreateAccountsReqNormalize(t *testing.T) {
	r := CreateAccountsReq{}
	if err := r.Normalize(); err != nil || r.Count != 1 || r.ValidityUnit != "day" {
		t.Fatalf("defaults: %+v %v", r, err)
	}
	bad := []CreateAccountsReq{
		{Count: MaxBatchCreate + 1},
		{Count: -1},
		{Username: "a"},
		{Validity: -1},
		{Validity: 1, ValidityUnit: "year"},
	}
	for _, b := range bad {
		if err := b.Normalize(); err == nil {
			t.Errorf("expected error for %+v", b)
		}
	}
	ok := CreateAccountsReq{Username: " vip ", Count: 50, Validity: 3, ValidityUnit: "month"}
	if err := ok.Normalize(); err != nil || ok.Username != "vip" {
		t.Fatalf("ok: %+v %v", ok, err)
	}
}
