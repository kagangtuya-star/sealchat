package utils

import (
	"net"
	"strings"
)

// NormalizeTencentCOSHost removes an optional bucket label from a Tencent COS
// endpoint. COS virtual-host requests add the configured bucket themselves;
// retaining a bucket-specific endpoint would otherwise produce a wrong host.
func NormalizeTencentCOSHost(rawHost string) (string, bool) {
	host := strings.TrimSpace(strings.TrimSuffix(rawHost, "."))
	if host == "" {
		return host, false
	}

	name, port, err := net.SplitHostPort(host)
	if err != nil {
		name = host
	}
	labels := strings.Split(name, ".")
	if len(labels) < 4 ||
		!strings.EqualFold(labels[len(labels)-2], "myqcloud") ||
		!strings.EqualFold(labels[len(labels)-1], "com") ||
		!strings.EqualFold(labels[len(labels)-4], "cos") {
		return rawHost, false
	}

	// cos.<region>.myqcloud.com: endpoint root.
	// <bucket>.cos.<region>.myqcloud.com: bucket-specific endpoint.
	if len(labels) == 5 {
		name = strings.Join(labels[1:], ".")
	} else if len(labels) != 4 {
		return rawHost, false
	}
	if port != "" {
		name = net.JoinHostPort(name, port)
	}
	return name, true
}
