package proxy

import (
	"io"
	"net/http"
	"strconv"
	"strings"
)

func readBody(body *io.ReadCloser) string {
	if body == nil || *body == nil {
		return ""
	}
	defer (*body).Close()
	b, err := io.ReadAll(io.LimitReader(*body, maxBody))
	if err != nil {
		return ""
	}
	return string(b)
}

func stripPort(host string) string {
	if i := strings.LastIndexByte(host, ':'); i > 0 {
		// Avoid stripping inside IPv6 literals like [::1] without a port.
		if !strings.Contains(host[i:], "]") {
			return host[:i]
		}
	}
	return host
}

func fullURL(scheme, host, uri string) string {
	if strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
		return uri
	}
	return scheme + "://" + host + uri
}

func headerMap(h http.Header) map[string]string {
	m := make(map[string]string, len(h))
	for k, vs := range h {
		m[k] = strings.Join(vs, ", ")
	}
	return m
}

func applyHeaderMap(h http.Header, m map[string]string) {
	for k, v := range m {
		h.Set(k, v)
	}
}

func copyHeaders(dst, src http.Header) {
	for k, vs := range src {
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

func rewriteHeaders(h http.Header, rs *RuleSet, target string) {
	for k, vs := range h {
		for i, v := range vs {
			line := k + ": " + v
			if nl, changed := rs.apply(target, line); changed {
				if idx := strings.Index(nl, ": "); idx >= 0 {
					nk := nl[:idx]
					nv := nl[idx+2:]
					if nk == k {
						vs[i] = nv
					} else {
						h.Del(k)
						h.Set(nk, nv)
					}
				}
			}
		}
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
