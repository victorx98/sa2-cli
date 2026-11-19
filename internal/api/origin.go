package api

import (
	"errors"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var ErrInvalidOrigin = errors.New("invalid HTTPS origin")

var dnsLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func Origin(value string) (string, error) {
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil ||
		strings.ContainsAny(value, "?#") || (u.Path != "" && u.Path != "/") ||
		u.RawPath != "" || strings.HasSuffix(u.Host, ":") {
		return "", ErrInvalidOrigin
	}

	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if addr, err := netip.ParseAddr(host); err == nil && addr.Zone() == "" {
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
		if number != 443 {
			host += ":" + strconv.FormatUint(number, 10)
		}
	}
	return "https://" + host, nil
}
