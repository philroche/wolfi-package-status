// Author: Phil Roche - phil.roche@chainguard.dev
// Date: 20240808
// This is a simple CLI tool that lists the latest version of a given package across all wolfi repositories
// The tool takes a list of package names as arguments and returns the latest version of each package
// The tool also supports regex matching of package names
// The tool also supports the `--help` flag to display usage information
// The tool uses the go-humanize library to format the build time of the package
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime"
	"strings"

	"github.com/dustin/go-humanize"
	"gitlab.alpinelinux.org/alpine/go/repository"
	"golang.org/x/sync/errgroup"
)

func main() {
	var (
		matchAsRegex          bool
		listAllVersions       bool
		showParentPkgInfo     bool
		showSubPkgInfo        bool
		helpTxt               bool
		outputJSON            bool
		localAPKIndex         string
		localAuthToken        string
		httpBasicAuthPassword string
	)

	flag.BoolVar(&matchAsRegex, "regex", false, "Parse package names as regex")
	flag.BoolVar(&listAllVersions, "all-versions", false, "List all matching package versions - not only the latest")
	flag.StringVar(&localAPKIndex, "local-apkindex", "", "Path to a local APKINDEX file")
	flag.StringVar(&localAuthToken, "auth-token", "", "Specify auth token to use when querying non public wolfi package repositories - enterprise-packages and extra-packages - use $(chainctl auth token --audience apk.cgr.dev). You can also set environment variable HTTP_AUTH.")
	flag.BoolVar(&showParentPkgInfo, "show-parent-package", false, "This might be a sub package of a parent package, show the parent package information")
	flag.BoolVar(&showSubPkgInfo, "show-sub-packages", false, "Show the sub package information. This will only take effect when a non regex package name filter is used.")
	flag.BoolVar(&helpTxt, "help", false, "Display usage information")
	flag.BoolVar(&outputJSON, "json", false, "Render output in JSON format")
	flag.Parse()

	if len(os.Args) < 2 || helpTxt {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s: [options] [package names]\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	httpBasicAuthPassword = getEnvWithFallback("HTTP_AUTH", localAuthToken)
	if httpBasicAuthPassword == "" {
		fmt.Fprintf(os.Stderr, "Specifying an auth token is required. Use `chainctl auth token --audience apk.cgr.dev` to get the required token. Please enter token now - alternatively, you can also specify this via --auth-token flag or by setting HTTP_AUTH environment variable: ")
		r, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			fmt.Fprintf(os.Stderr, err.Error())
			os.Exit(1)
		}
		httpBasicAuthPassword = strings.Trim(r, "\r\n")
	}

	var queries []Matcher
	var err error
	args := flag.Args()
	if len(args) > 0 {
		for _, arg := range args {
			var q Matcher
			if matchAsRegex {
				q, err = regexp.Compile(arg)
				if err != nil {
					fmt.Fprintf(os.Stderr, "failed to parse regexp from input query %s: %w", arg, err)
					os.Exit(1)
				}
			} else {
				q = NewQueryString(arg)
			}
			queries = append(queries, q)
		}
	}

	var APKIndexURLs map[APKIndex]string

	if len(localAPKIndex) != 0 {
		APKIndexURLs = map[APKIndex]string{
			LOCAL: localAPKIndex,
		}
	} else {
		APKIndexURLs = DefaultAPKIndices
	}

	result := &PackageInfoOutput{}
	var g errgroup.Group
	g.SetLimit(runtime.NumCPU())
	for indexName, url := range APKIndexURLs {
		indexName, url := indexName, url
		g.Go(func() error {
			reader, err := fetchAPKIndex(url, httpBasicAuthPassword)
			if err != nil {
				return err
			}
			defer reader.Close()
			apkIndex, err := repository.IndexFromArchive(reader)
			if err != nil {
				return fmt.Errorf("failed to read APKINDEX archive %s: %w", url, err)
			}
			packages := apkIndex.Packages
			for _, pkg := range packages {
				if len(queries) > 0 {
					result.AddPackageMeta(queries, pkg, string(indexName))
				} else {
					// we are not matching any packages here so print all found package names and versions
					parentPkgInfo := ""
					if showParentPkgInfo {
						parentPkgInfo = " - Parent/Origin package: " + pkg.Origin
					}
					fmt.Printf("%s version %s (%s - %s) in %s repository%s\n", pkg.Name, pkg.Version, humanize.Time(pkg.BuildTime), pkg.BuildTime, indexName, parentPkgInfo)
				}
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to fetch package info from index: %w", err)
	}

	result.Sort()
	result.Print(listAllVersions, outputJSON, showParentPkgInfo, showSubPkgInfo)
}
