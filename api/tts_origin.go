package api

import (
	"net"
	"net/url"
	"strconv"
	"strings"
)

func ttsOriginMatchesHost(originHeader, requestHost string) bool {
	origin, err := url.Parse(originHeader)
	if err != nil || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Host == "" || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		return false
	}
	originHost, ok := ttsCanonicalHost(origin.Host, origin.Scheme)
	if !ok {
		return false
	}
	host, ok := ttsCanonicalHost(requestHost, origin.Scheme)
	return ok && originHost == host
}

func ttsCanonicalHost(rawHost, scheme string) (string, bool) {
	u, err := url.Parse("//" + rawHost)
	if err != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", false
	}
	port := 80
	if scheme == "https" {
		port = 443
	}
	if rawPort := u.Port(); rawPort != "" {
		port, err = strconv.Atoi(rawPort)
		if err != nil || port < 1 || port > 65535 {
			return "", false
		}
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), true
}
