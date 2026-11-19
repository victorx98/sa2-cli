package api

import (
	"errors"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var ErrInvalidOrigin = errors.New("invalid API origin")

var dnsLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func Origin(value string) (string, error) {
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil ||
		strings.ContainsAny(value, "?#") || (u.Path != "" && u.Path != "/") ||
		u.RawPath != "" || strings.HasSuffix(u.Host, ":") {
		return "", ErrInvalidOrigin
	}

	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	addr, addrErr := netip.ParseAddr(host)
	if u.Scheme == "http" && host != "localhost" && !addr.IsLoopback() {
		return "", ErrInvalidOrigin
	}
	if addrErr == nil && addr.Zone() == "" {
		host = addr.String()
		if addr.Is6() {
			host = "[" + host + "]"
		}
	} else {
		if strings.HasPrefix(u.Host, "[") || len(host) > 253 {
			return "", ErrInvalidOrigin
		}
		for _, label := range strings.Split(host, ".") {
			if !dnsLabel.MatchString(label) {
				return "", ErrInvalidOrigin
			}
		}
	}

	if port := u.Port(); port != "" {
		number, err := strconv.ParseUint(port, 10, 16)
		if err != nil || number == 0 {
			return "", ErrInvalidOrigin
		}
		defaultPort := uint64(443)
		if u.Scheme == "http" {
			defaultPort = 80
		}
		if number != defaultPort {
			host += ":" + strconv.FormatUint(number, 10)
		}
	}
	return u.Scheme + "://" + host, nil
}
