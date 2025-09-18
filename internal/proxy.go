package internal

import (
	"strconv"
	"strings"
)

// Proxy represents a parsed proxy entry.
type Proxy struct {
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Passwd   string `json:"passwd"`
	Opaque   string `json:"opaque"`
	Protocol string `json:"protocol"`
}

func (p *Proxy) String() string {
	if p.Opaque != "" {
		return strings.ToLower(p.Protocol) + "://" + p.Opaque
	}

	if p.User == "" {
		return strings.ToLower(p.Protocol) + "://" + p.IP + ":" + strconv.Itoa(p.Port)
	}

	if p.Passwd == "" {
		return strings.ToLower(p.Protocol) + "://" + p.User + "@" + p.IP + ":" + strconv.Itoa(p.Port)
	}

	return strings.ToLower(p.Protocol) + "://" + p.User + ":" + p.Passwd + "@" + p.IP + ":" + strconv.Itoa(p.Port)
}
