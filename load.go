package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

type Scheme string

var (
	client = &http.Client{
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

	HTTPS Scheme = "https"
	HTTP  Scheme = "https"
	FILE  Scheme = "file"
)

func parseScheme(ref string) (Scheme, error) {
	parts := strings.SplitAfterN(ref, "://", 2)
	if len(parts) == 2 {
		sch := parts[0]
		switch sch {
		case "http://":
			return HTTP, nil
		case "https://":
			return HTTPS, nil
		default:
			// TODO: add OCI Support?
			return "", fmt.Errorf("unknown ref scheme found")
		}
	} else {
		return FILE, nil
	}
}

func fetchAPKIndex(ref, httpBasicAuthPassword string) (io.ReadCloser, error) {
	scheme, err := parseScheme(ref)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch APKIndex data: %w", err)
	}
	switch scheme {
	case HTTPS:
		req, err := http.NewRequest("GET", ref, nil)
		if err != nil {
			return nil, fmt.Errorf("Error creating request: %w", err)
		}

		// Add the auth token to the request header but only for non public repositories
		if httpBasicAuthPassword != "" { // TODO: why not just pass it for public repo as well
			encodedAuth := base64.StdEncoding.EncodeToString([]byte("user:" + httpBasicAuthPassword))
			req.Header.Set("Authorization", "Basic "+encodedAuth)
		}
		req.Header.Set("Accept", "application/gzip")
		req.Header.Add("User-Agent", UserAgent) // TODO: does this only work with curl??

		// Send the request via a client
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("Failed to download APKINDEX file %s: %w\n", ref, err)
		}

		return resp.Body, nil
	case FILE:
		file, err := os.Open(ref)
		if err != nil {
			return nil, fmt.Errorf("failed to open APKINDEX file %s: %w", ref, err)
		}
		return file, nil
	default:
		return nil, fmt.Errorf("%s scheme not support for apk index reference", scheme)
	}
}
