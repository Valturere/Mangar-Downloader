//go:build windows

package updates

import (
	"golang.org/x/sys/windows"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
	"unsafe"
)

var winHTTP = windows.NewLazySystemDLL("winhttp.dll")

type proxyConfig struct {
	AutoDetect             int32
	AutoURL, Proxy, Bypass *uint16
}
type proxyOptions struct {
	Flags, DetectFlags uint32
	Config             *uint16
	Reserved           unsafe.Pointer
	ReservedFlags      uint32
	AutoLogon          int32
}
type proxyInfo struct {
	AccessType    uint32
	Proxy, Bypass *uint16
}

func freeProxyPointers(pointers ...*uint16) {
	for _, p := range pointers {
		if p != nil {
			windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalFree").Call(uintptr(unsafe.Pointer(p)))
		}
	}
}
func text16(p *uint16) string {
	if p == nil {
		return ""
	}
	return windows.UTF16PtrToString(p)
}
func selectProxy(request *http.Request, server, bypass string) (*url.URL, error) {
	host := strings.ToLower(request.URL.Hostname())
	for _, rule := range strings.FieldsFunc(strings.ToLower(bypass), func(c rune) bool { return c == ';' || c == ',' || c == ' ' }) {
		if rule == "<local>" && !strings.Contains(host, ".") {
			return nil, nil
		}
		if match, _ := path.Match(rule, host); match {
			return nil, nil
		}
	}
	selected := ""
	for _, part := range strings.FieldsFunc(server, func(c rune) bool { return c == ';' || c == ' ' }) {
		if kind, value, ok := strings.Cut(part, "="); ok {
			if strings.EqualFold(kind, request.URL.Scheme) {
				selected = value
				break
			}
		} else if selected == "" {
			selected = part
		}
	}
	if selected == "" {
		return nil, nil
	}
	if !strings.Contains(selected, "://") {
		selected = "http://" + selected
	}
	p, e := url.Parse(selected)
	if e != nil || p.Hostname() == "" || (p.Scheme != "http" && p.Scheme != "https") {
		return nil, ErrNetwork
	}
	return p, nil
}

// Explicit environment proxy settings take precedence; otherwise use the current
// user's Windows connection configuration, including per-URL PAC/WPAD resolution.
func systemProxy(request *http.Request) (*url.URL, error) {
	if request.Context().Err() != nil {
		return nil, request.Context().Err()
	}
	if os.Getenv("HTTPS_PROXY") != "" || os.Getenv("https_proxy") != "" || os.Getenv("HTTP_PROXY") != "" || os.Getenv("http_proxy") != "" {
		return http.ProxyFromEnvironment(request)
	}
	type result struct {
		url *url.URL
		err error
	}
	resolved := make(chan result, 1)
	go func() { p, e := windowsProxy(request); resolved <- result{p, e} }()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	select {
	case r := <-resolved:
		return r.url, r.err
	case <-request.Context().Done():
		return nil, request.Context().Err()
	case <-deadline.C:
		return nil, ErrNetwork
	}
}

// WinHTTP owns a short-lived session. Its synchronous PAC resolver can finish
// in the background after cancellation; all handles are released here.
func windowsProxy(request *http.Request) (*url.URL, error) {
	var config proxyConfig
	ok, _, callErr := winHTTP.NewProc("WinHttpGetIEProxyConfigForCurrentUser").Call(uintptr(unsafe.Pointer(&config)))
	if ok == 0 {
		if callErr == windows.ERROR_FILE_NOT_FOUND {
			return nil, nil
		}
		return nil, ErrNetwork
	}
	defer freeProxyPointers(config.AutoURL, config.Proxy, config.Bypass)
	if config.AutoURL != nil || config.AutoDetect != 0 {
		agent, _ := windows.UTF16PtrFromString("Mangar Downloader")
		session, _, _ := winHTTP.NewProc("WinHttpOpen").Call(uintptr(unsafe.Pointer(agent)), 1, 0, 0, 0)
		if session == 0 {
			return nil, ErrNetwork
		}
		defer winHTTP.NewProc("WinHttpCloseHandle").Call(session)
		winHTTP.NewProc("WinHttpSetTimeouts").Call(session, 5000, 5000, 5000, 5000)
		address, _ := windows.UTF16PtrFromString(request.URL.String())
		options := proxyOptions{}
		if config.AutoURL != nil {
			options.Flags = 2
			options.Config = config.AutoURL
		} else {
			options.Flags = 1
			options.DetectFlags = 3
		}
		var info proxyInfo
		ok, _, _ = winHTTP.NewProc("WinHttpGetProxyForUrl").Call(session, uintptr(unsafe.Pointer(address)), uintptr(unsafe.Pointer(&options)), uintptr(unsafe.Pointer(&info)))
		defer freeProxyPointers(info.Proxy, info.Bypass)
		if ok != 0 {
			if info.AccessType == 1 {
				return nil, nil
			}
			return selectProxy(request, text16(info.Proxy), text16(info.Bypass))
		}
		if config.AutoURL != nil {
			return nil, ErrNetwork
		}
	}
	return selectProxy(request, text16(config.Proxy), text16(config.Bypass))
}
