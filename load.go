package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type Scheme string

var (
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

func fetchAPKIndex(index APKIndex, ref, httpBasicAuthPassword string) (io.ReadCloser, error) {
	scheme, err := parseScheme(ref)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch APKIndex data: %w", err)
	}
	switch scheme {
	case HTTPS:
		req, err := http.NewRequest("GET", ref, nil)
		if err != nil {
			return nil, fmt.Errorf("error creating request: %w", err)
		}

		// Add the auth token to the request header but only for non public repositories
		if httpBasicAuthPassword != "" && index != WOLFI {
			encodedAuth := base64.StdEncoding.EncodeToString([]byte("user:" + httpBasicAuthPassword))
			req.Header.Set("Authorization", "Basic "+encodedAuth)
		}
		req.Header.Set("Accept", "application/gzip")
		req.Header.Add("User-Agent", UserAgent)

		// Send the request via a client
		resp, err := DefaultHTTPClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to download APKINDEX file %s: %w", ref, err)
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
