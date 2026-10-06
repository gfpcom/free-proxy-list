package internal

import (
	"net"
	"strconv"
	"strings"
)

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

	address := net.JoinHostPort(p.IP, strconv.Itoa(p.Port))
	if p.User == "" {
		return strings.ToLower(p.Protocol) + "://" + address
	}

	if p.Passwd == "" {
		return strings.ToLower(p.Protocol) + "://" + p.User + "@" + address
	}

	return strings.ToLower(p.Protocol) + "://" + p.User + ":" + p.Passwd + "@" + address
}
