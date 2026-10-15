//go:build windows

package catagent

import (
	"golang.org/x/sys/windows/registry"
	"strings"
)

func cliSystemProxy() string {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	on, _, _ := k.GetIntegerValue("ProxyEnable")
	if on == 0 {
		return ""
	}
	p, _, _ := k.GetStringValue("ProxyServer")
	if strings.Contains(p, "=") {
		var fallback string
		for _, entry := range strings.Split(p, ";") {
			key, value, _ := strings.Cut(strings.TrimSpace(entry), "=")
			if key == "https" {
				return proxyHTTPURL(value)
			}
			if key == "http" {
				fallback = value
			}
		}
		p = fallback
	}
	return proxyHTTPURL(p)
}

func proxyHTTPURL(p string) string {
	p = strings.TrimSpace(p)
	if p != "" && !strings.Contains(p, "://") {
		p = "http://" + p
	}
	return p
}
