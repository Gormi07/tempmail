package store

import (
	"context"
	"errors"
	"net"
	"strings"
)

// MXCheckServerIPs 返回 MX 校验时可接受的服务器 IP 列表（逗号分隔）。
//
// 后台「系统设置」中的 smtp_server_ip 与环境变量 SMTP_SERVER_IP 都会被接受，
// DB 值在前。以前手动添加/注册走「DB 优先」，而后台 30 秒重试与 6 小时巡检
// 走「环境变量优先」，两边 IP 不一致时会出现：添加时立即激活、巡检后被降级为
// pending、之后的重试永远无法通过。所有 MX 校验路径必须统一使用本函数。
func (s *Store) MXCheckServerIPs(ctx context.Context, envIP string) string {
	var candidates []string
	if s != nil {
		if dbIP, err := s.GetSetting(ctx, "smtp_server_ip"); err == nil {
			candidates = append(candidates, dbIP)
		}
	}
	candidates = append(candidates, envIP)

	seen := make(map[string]bool)
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		for ip := range parseServerIPs(c) {
			if !seen[ip] {
				seen[ip] = true
				out = append(out, ip)
			}
		}
	}
	return strings.Join(out, ",")
}

// parseServerIPs 解析逗号/分号/空白分隔的 IP 列表，并去除首尾空白。
func parseServerIPs(raw string) map[string]bool {
	ips := make(map[string]bool)
	for _, f := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	}) {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if ip := net.ParseIP(f); ip != nil {
			ips[ip.String()] = true
		} else {
			ips[f] = true
		}
	}
	return ips
}

// isDefinitiveDNSMiss 只有 NXDOMAIN / 无记录 才算确定性失败；
// 超时、SERVFAIL、网络错误等均视为临时故障。
func isDefinitiveDNSMiss(err error) bool {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return dnsErr.IsNotFound && !dnsErr.IsTimeout && !dnsErr.IsTemporary
	}
	return false
}

// MergeCapabilities 根据本次检测结果计算新的能力值。
// 某一项检测遇到临时 DNS 故障时沿用原值，避免一次抖动就把已激活域名降级；
// 单域名与通配子域名两项互相独立，通配失败不会影响单域名能力。
func MergeCapabilities(prevSingle, prevWildcard bool, details []MXCheckDetail) (supportsSingle, supportsWildcard bool) {
	supportsSingle, supportsWildcard = prevSingle, prevWildcard
	for _, detail := range details {
		var next bool
		switch {
		case detail.Matched:
			next = true
		case detail.Transient:
			continue
		default:
			next = false
		}
		switch detail.Kind {
		case "single", "exact", "base":
			supportsSingle = next
		case "wildcard":
			supportsWildcard = next
		}
	}
	return supportsSingle, supportsWildcard
}
