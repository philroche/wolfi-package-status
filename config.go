package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"time"
)

var (
	DefaultHTTPClient = &http.Client{
		Timeout: 2 * time.Minute,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 5 * time.Minute,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          10,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
	// TODO: is there a way to get current wolfi-package-status version?
	UserAgent = fmt.Sprintf("wolfi-package-status/(%s; %s)", runtime.GOOS, runtime.GOARCH)

	WriteStream io.ReadWriter = os.Stdout
	ErrorStream io.ReadWriter = os.Stderr
	InputStream io.ReadWriter = os.Stdin

	DefaultAPKIndices = map[APKIndex]string{
		WOLFI:               "https://packages.wolfi.dev/os/x86_64/APKINDEX.tar.gz",
		ENTERPRISE_PACKAGES: "https://apk.cgr.dev/chainguard-private/x86_64/APKINDEX.tar.gz",
		EXTRA_PACKAGES:      "https://apk.cgr.dev/extra-packages/x86_64/APKINDEX.tar.gz",
	}
)

