// Author: Phil Roche - phil.roche@chainguard.dev
// Date: 20240808
// This is a simple CLI tool that lists the latest version of a given package across all wolfi repositories
// The tool takes a list of package names as arguments and returns the latest version of each package
// The tool also supports regex matching of package names
// The tool also supports the `--help` flag to display usage information
// The tool uses the go-humanize library to format the build time of the package
package main

import (
	"flag"
	"fmt"
	"github.com/dustin/go-humanize"
	"gitlab.alpinelinux.org/alpine/go/repository"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

func isValidRegex(pattern string) bool {
	_, err := regexp.Compile(pattern)
	return err == nil
}

func matchRegex(s string, pattern string) bool {
	regex, err := regexp.Compile(pattern)
	if err != nil {
		log.Fatalf("Invalid regex pattern: %v", err)
	}
	return regex.MatchString(s)
}

func main() {
	// Parse the command line arguments
	matchAsRegex := flag.Bool("regex", false, "Parse package names as regex")
	listAllVersions := flag.Bool("all-versions", false, "List all matching package versions - not only the latest")
	helpText := flag.Bool("help", false, "Display usage information")
	flag.Parse()

	if *helpText {
		fmt.Printf("Usage: %s [options] [package names]\n", os.Args[0])
		fmt.Println("\t* Mulitple package names can be specified separated by space")
		fmt.Println("\t* Option `--regex` can be used to match package names on specified regular expression. Multiple regular expressions can be specified separated by space")
		fmt.Println("\t* Option `--all-versions` can be used to list all package versions, not only the latest.")
		fmt.Println("\t* Option `--help` can be used to display this usage message")
		os.Exit(0)
	}
	packageNames := []string{}
	args := os.Args
	if len(args) > 1 {
		for i := 1; i < len(args); i++ {
			packageNames = append(packageNames, args[i])
		}
	}
	var APKINDEXURLs = make(map[string]string)
	APKINDEXURLs["wolfi os"] = "https://packages.wolfi.dev/os/x86_64/APKINDEX.tar.gz"
	APKINDEXURLs["enterprise packages"] = "https://packages.cgr.dev/os/x86_64/APKINDEX.tar.gz"
	APKINDEXURLs["extra packages"] = "https://packages.cgr.dev/extras/x86_64/APKINDEX.tar.gz"

	var matchingPackagesAllVersions = make(map[string][]map[string]interface{})
	var matchingPackagesLatestVersion = make(map[string]map[string]interface{})
	//for each of the APKINDEXURLs create an instance of the repository class
	for APKINDEXFriendlyName, APKINDEXurl := range APKINDEXURLs {
		// Download each of the APKINDEX files to temporary directory using "net/http"
		resp, err := http.Get(APKINDEXurl)
		if err != nil {
			fmt.Printf("Failed to download APKINDEX file %s: %v\n", APKINDEXurl, err)
			os.Exit(1)
		}
		// write the response variable resp to a file in a temporary directory
		defer resp.Body.Close()
		// Create a temporary directory
		temporaryAPKINDEXdir, err := os.MkdirTemp("", "wolfi-package-status")
		if err != nil {
			fmt.Printf("Failed to create temporary directory %s: %v\n", temporaryAPKINDEXdir, err)
			os.Exit(1)
		}

		// Create a file in the temporary directory
		localAPKINDEXPath := filepath.Join(temporaryAPKINDEXdir, "APKINDEX.tar.gz")
		localAPKINDEXfile, err := os.Create(localAPKINDEXPath)
		if err != nil {
			fmt.Printf("Failed to write APKINDEX file to temporary directory %s: %v\n", temporaryAPKINDEXdir, err)
			os.Exit(1)
		}
		defer localAPKINDEXfile.Close()

		// Write the response to file
		_, err = io.Copy(localAPKINDEXfile, resp.Body)
		// fmt.Println(localAPKINDEXPath)
		indexFile, err := os.Open(localAPKINDEXPath)
		apkIndex, err := repository.IndexFromArchive(indexFile)
		// fmt.Println(len(apkIndex.Packages))
		packages := apkIndex.Packages

		for _, _package := range packages {
			// fmt.Println(_package.Name)
			if len(packageNames) > 0 {
				for _, packageName := range packageNames {
					var matchFound bool
					if packageName == _package.Name {
						matchFound = true
					}
					if *matchAsRegex && isValidRegex(packageName) && matchRegex(_package.Name, packageName) {
						matchFound = true
					}

					if matchFound {
						// if this is the first time we have encountered this package - ensure the inner map is initialized
						matchingPackagesLatestVersionInnerMap := matchingPackagesLatestVersion[_package.Name]
						if matchingPackagesLatestVersionInnerMap == nil {
							matchingPackagesLatestVersionInnerMap = make(map[string]interface{})
							matchingPackagesLatestVersion[_package.Name] = matchingPackagesLatestVersionInnerMap
						}

						// if this is the first time we have encountered this package - ensure the inner map is initialized
						matchingPackagesAllVersionsInnerMap := matchingPackagesAllVersions[_package.Name]
						if matchingPackagesAllVersionsInnerMap == nil {
							matchingPackagesAllVersionsInnerMap = []map[string]interface{}{}
							matchingPackagesAllVersions[_package.Name] = matchingPackagesAllVersionsInnerMap
						}
						// add the package to the list of all versions
						matchingPackagesAllVersions[_package.Name] = append(matchingPackagesAllVersions[_package.Name], map[string]interface{}{"Version": _package.Version, "BuildTime": _package.BuildTime, "Repository": APKINDEXFriendlyName})

						// Now check to see if this is the latest version
						latestVersion, latestVersionFound := matchingPackagesLatestVersion[_package.Name]["Version"].(string)
						if !latestVersionFound || latestVersion == "" || _package.Version > latestVersion {
							matchingPackagesLatestVersion[_package.Name]["Version"] = _package.Version
							matchingPackagesLatestVersion[_package.Name]["BuildTime"] = _package.BuildTime
							matchingPackagesLatestVersion[_package.Name]["Repository"] = APKINDEXFriendlyName
						}
					}
				}
			} else {
				// we are not matching any packages here so print all found package names and versions
				fmt.Printf("%s version %s (%s - %s) in %s repository\n", _package.Name, _package.Version, humanize.Time(_package.BuildTime), _package.BuildTime, APKINDEXFriendlyName)
			}

		}
		// delete the temporary directory
		err = os.RemoveAll(temporaryAPKINDEXdir)
		if err != nil {
			fmt.Printf("Unable to delete temporary directory %s: %v\n", temporaryAPKINDEXdir, err)
			os.Exit(1)
		}
	}
	if *listAllVersions {
		// we want to order the output by package name
		packageNameKeys := make([]string, 0, len(matchingPackagesAllVersions))
		for k := range matchingPackagesAllVersions {
			packageNameKeys = append(packageNameKeys, k)
		}
		// Sort the keys.
		sort.Strings(packageNameKeys)
		for _, matchingPackageAllVersionsPackageName := range packageNameKeys {
			matchingPackageMap := matchingPackagesAllVersions[matchingPackageAllVersionsPackageName]
			fmt.Printf("The versions of package %s are:\n", matchingPackageAllVersionsPackageName)
			for _, versionMap := range matchingPackageMap {
				fmt.Printf("%s (%s - %s) in %s repository\n", versionMap["Version"].(string), humanize.Time(versionMap["BuildTime"].(time.Time)), versionMap["BuildTime"].(time.Time), versionMap["Repository"].(string))
			}
		}
	} else {
		// we want to order the output by package name
		packageNameKeys := make([]string, 0, len(matchingPackagesLatestVersion))
		for k := range matchingPackagesLatestVersion {
			packageNameKeys = append(packageNameKeys, k)
		}
		// Sort the keys.
		sort.Strings(packageNameKeys)
		for _, matchingPackageLatestVersionPackageName := range packageNameKeys {
			matchingPackageMap := matchingPackagesLatestVersion[matchingPackageLatestVersionPackageName]
			fmt.Printf("The latest version of package %s is %s (%s - %s) in %s repository\n", matchingPackageLatestVersionPackageName, matchingPackageMap["Version"].(string), humanize.Time(matchingPackageMap["BuildTime"].(time.Time)), matchingPackageMap["BuildTime"].(time.Time), matchingPackageMap["Repository"].(string))
		}
	}
}
