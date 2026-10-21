package store

import (
	"errors"
	"net"
	"testing"
)

func TestParseServerIPs(t *testing.T) {
	ips := parseServerIPs(" 177.2.184.135 , 10.0.0.1;  ")
	if !ips["177.2.184.135"] || !ips["10.0.0.1"] || len(ips) != 2 {
		t.Fatalf("unexpected ips: %v", ips)
	}
	if len(parseServerIPs("   ")) != 0 {
		t.Fatal("blank input must yield no ips")
	}
}

func TestMXCheckServerIPsNilStoreUsesEnv(t *testing.T) {
	var s *Store
	if got := s.MXCheckServerIPs(nil, " 1.2.3.4 "); got != "1.2.3.4" {
		t.Fatalf("got %q", got)
	}
}

func TestIsDefinitiveDNSMiss(t *testing.T) {
	if !isDefinitiveDNSMiss(&net.DNSError{IsNotFound: true}) {
		t.Fatal("NXDOMAIN must be definitive")
	}
	if isDefinitiveDNSMiss(&net.DNSError{IsTimeout: true}) {
		t.Fatal("timeout must be transient")
	}
	if isDefinitiveDNSMiss(&net.DNSError{IsTemporary: true}) {
		t.Fatal("SERVFAIL must be transient")
	}
	if isDefinitiveDNSMiss(errors.New("boom")) {
		t.Fatal("unknown errors must be transient")
	}
}

func TestMergeCapabilities(t *testing.T) {
	single := MXCheckDetail{Kind: "single", Matched: true}
	wildMiss := MXCheckDetail{Kind: "wildcard"}
	// 通配确定性失败不影响单域名
	if s, w := MergeCapabilities(true, false, []MXCheckDetail{single, wildMiss}); !s || w {
		t.Fatalf("got single=%v wildcard=%v", s, w)
	}
	// 单域名临时故障沿用原值
	if s, _ := MergeCapabilities(true, false, []MXCheckDetail{{Kind: "single", Transient: true}, wildMiss}); !s {
		t.Fatal("transient failure must keep previous single capability")
	}
	// 单域名确定性失败才降级
	if s, _ := MergeCapabilities(true, false, []MXCheckDetail{{Kind: "single"}, wildMiss}); s {
		t.Fatal("definitive failure must clear single capability")
	}
	// pending 域名（原值 false）匹配后可提升
	if s, _ := MergeCapabilities(false, false, []MXCheckDetail{single, wildMiss}); !s {
		t.Fatal("pending domain must be promotable")
	}
}

func TestCheckMXNameNoServerIPIsTransient(t *testing.T) {
	d := checkMXName("single", "example.invalid", "")
	if d.Matched || !d.Transient {
		t.Fatalf("unexpected detail: %+v", d)
	}
}
