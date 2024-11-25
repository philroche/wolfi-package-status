// Author: Phil Roche - phil.roche@chainguard.dev
// Date: 20240808
// This is a simple CLI tool that lists the latest version of a given package across all wolfi repositories
// The tool takes a list of package names as arguments and returns the latest version of each package
// The tool also supports regex matching of package names
// The tool also supports the `--help` flag to display usage information
// The tool uses the go-humanize library to format the build time of the package
package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"github.com/dustin/go-humanize"
	"github.com/knqyf263/go-apk-version"
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

// getEnvOrFlag returns the value of the environment variable if it is set, otherwise returns the command-line flag value.
func getEnvOrFlag(envName string, flagValue *string) string {
	if value, exists := os.LookupEnv(envName); exists {
		return value
	}
	return *flagValue
}

func removeDuplicates(stringsList []string) []string {
	// Create a map to track seen elements
	seen := make(map[string]struct{})
	// Create a new slice to store the result
	var result []string

	// Iterate over the original slice
	for _, item := range stringsList {
		// If the item is not in the map, add it
		if _, found := seen[item]; !found {
			seen[item] = struct{}{}
			result = append(result, item)
		}
	}

	return result
}

func main() {
	// Parse the command line arguments
	matchAsRegex := flag.Bool("regex", false, "Parse package names as regex")
	listAllVersions := flag.Bool("all-versions", false, "List all matching package versions - not only the latest")
	localAPKINDEX := flag.String("local-apkindex", "", "Path to a local APKINDEX file")
	localAuthToken := flag.String("auth-token", "", "Specify auth token to use when querying non public wolfi package repositories - enterprise-packages and extra-packages - use $(chainctl auth token --audience apk.cgr.dev). You can also set environment variable HTTP_AUTH.")
	showParentPackageInformation := flag.Bool("show-parent-package", false, "This might be a sub package of a parent package, show the parent package information")
	showSubPackageInformation := flag.Bool("show-sub-packages", false, "Show the sub package information. This will only take effect when a non regex package name filter is used.")
	helpText := flag.Bool("help", false, "Display usage information")
	flag.Parse()
	httpBasicAuthPassword := getEnvOrFlag("HTTP_AUTH", localAuthToken)
	// if the httpBasicAuthPassword is not set, then we need to prompt the user for it
	if httpBasicAuthPassword == "" {
		fmt.Print("Specifying an auth token is required. Use `chainctl auth token --audience apk.cgr.dev` to get the required token. Please enter token now - alternatively, you can also specify this via --auth-token flag or by setting HTTP_AUTH environment variable: ")
		_, _ = fmt.Scanln(&httpBasicAuthPassword)
	}
	if *helpText {
		fmt.Printf("Usage: %s [options] [package names]\n", os.Args[0])
		fmt.Println("\t* Mulitple package names can be specified separated by space")
		fmt.Println("\t* Option `--regex` can be used to match package names on specified regular expression. Multiple regular expressions can be specified separated by space")
		fmt.Println("\t* Option `--all-versions` can be used to list all package versions, not only the latest.")
		fmt.Println("\t* Option `--auth-token` specify auth token to use when querying wolfi non public package repositories - enterprise-packages and extra-packages - use $(chainctl auth token --audience apk.cgr.dev). You can also set environment variable HTTP_AUTH.")
		fmt.Println("\t* Option `--show-parent-package` can be used to show the parent package information as the package being queried might be defined as a sub package.")
		fmt.Println("\t* Option `--show-sub-packages` can be used to show the sub package information as the package being queried might be defined as a parent/origin package. This will only take effect when a non regex package name filter is used.")
		fmt.Println("\t* Option `--local-apkindex` can be used to specify a local APKINDEX.tar.gz file to use instead of querying remote repositories.")
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

	if *localAPKINDEX != "" {
		APKINDEXURLs["local apkindex"] = *localAPKINDEX
	} else {
		APKINDEXURLs["wolfi os"] = "https://packages.wolfi.dev/os/x86_64/APKINDEX.tar.gz"
		APKINDEXURLs["enterprise packages"] = "https://apk.cgr.dev/chainguard-private/x86_64/APKINDEX.tar.gz"
		APKINDEXURLs["extra packages"] = "https://apk.cgr.dev/extra-packages/x86_64/APKINDEX.tar.gz"
	}

	var matchingPackagesAllVersions = make(map[string][]map[string]interface{})
	var matchingPackagesLatestVersion = make(map[string]map[string]interface{})
	var subPackageNames []string
	//for each of the APKINDEXURLs create an instance of the repository class
	for APKINDEXFriendlyName, APKINDEXurl := range APKINDEXURLs {
		// check to see of APKINDEXurl is a local file
		localAPKINDEXPath := ""
		temporaryAPKINDEXdir := ""
		if _, err := os.Stat(APKINDEXurl); err == nil {
			localAPKINDEXPath = APKINDEXurl
		} else {
			// Download each of the APKINDEX files to temporary directory using "net/http"

			// use localAuthToken if it is set when making request to non public repositories
			// Create a new request
			req, err := http.NewRequest("GET", APKINDEXurl, nil)
			if err != nil {
				fmt.Println("Error creating request:", err)
				return
			}

			// Add the auth token to the request header but only for non public repositories
			if httpBasicAuthPassword != "" && APKINDEXFriendlyName != "wolfi os" {
				encodedAuth := base64.StdEncoding.EncodeToString([]byte("user:" + httpBasicAuthPassword))
				req.Header.Set("Authorization", "Basic "+encodedAuth)
			}

			req.Header.Set("Accept", "application/gzip")
			req.Header.Add("User-Agent", "curl/7.68.0")

			// Send the request via a client
			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				fmt.Printf("Failed to download APKINDEX file %s: %v\n", APKINDEXurl, err)
				os.Exit(1)
			}

			// write the response variable resp to a file in a temporary directory
			defer resp.Body.Close()
			// Create a temporary directory
			temporaryAPKINDEXdir, err = os.MkdirTemp("", "wolfi-package-status")
			if err != nil {
				fmt.Printf("Failed to create temporary directory %s: %v\n", temporaryAPKINDEXdir, err)
				os.Exit(1)
			}

			// Create a file in the temporary directory
			localAPKINDEXPath = filepath.Join(temporaryAPKINDEXdir, "APKINDEX.tar.gz")
			localAPKINDEXfile, err := os.Create(localAPKINDEXPath)
			if err != nil {
				fmt.Printf("Failed to write APKINDEX file to temporary directory %s: %v\n", temporaryAPKINDEXdir, err)
				os.Exit(1)
			}
			defer localAPKINDEXfile.Close()

			// Write the response to file
			_, err = io.Copy(localAPKINDEXfile, resp.Body)
		}

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
						matchingPackagesAllVersions[_package.Name] = append(
							matchingPackagesAllVersions[_package.Name],
							map[string]interface{}{
								"Version":    _package.Version,
								"BuildTime":  _package.BuildTime,
								"Repository": APKINDEXFriendlyName,
								"Origin":     _package.Origin,
							},
						)

						// Now check to see if this is the latest version
						latestVersion, latestVersionFound := matchingPackagesLatestVersion[_package.Name]["Version"].(string)
						semver_latestVersion, _ := version.NewVersion(latestVersion)
						semver_packageVersion, _ := version.NewVersion(_package.Version)
						if !latestVersionFound || latestVersion == "" || semver_packageVersion.GreaterThan(semver_latestVersion) {
							matchingPackagesLatestVersion[_package.Name]["Version"] = _package.Version
							matchingPackagesLatestVersion[_package.Name]["BuildTime"] = _package.BuildTime
							matchingPackagesLatestVersion[_package.Name]["Repository"] = APKINDEXFriendlyName
							matchingPackagesLatestVersion[_package.Name]["Origin"] = _package.Origin
						}
					} else {
						if *showSubPackageInformation && !*matchAsRegex {
							//is there an origin of this package and if so does it match the package name filter
							if _package.Origin != "" && _package.Origin == packageName {
								subPackageNames = append(subPackageNames, _package.Name)
							}
						}
					}
				}
			} else {
				// we are not matching any packages here so print all found package names and versions
				_parentPackageInformation := ""
				if *showParentPackageInformation {
					_parentPackageInformation = " - Parent/Origin package: " + _package.Origin
				}
				fmt.Printf("%s version %s (%s - %s) in %s repository%s\n", _package.Name, _package.Version, humanize.Time(_package.BuildTime), _package.BuildTime, APKINDEXFriendlyName, _parentPackageInformation)
			}

		}

		// if it is not a local file, delete the temporary directory
		if localAPKINDEXPath != APKINDEXurl {
			// delete the temporary directory
			err = os.RemoveAll(temporaryAPKINDEXdir)
			if err != nil {
				fmt.Printf("Unable to delete temporary directory %s: %v\n", temporaryAPKINDEXdir, err)
				os.Exit(1)
			}
		}
	}
	// ensure if subPackageNames is not empty that we remove any duplicates
	subPackageNames = removeDuplicates(subPackageNames)

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
			// Sort the version maps by Version in ascending order
			sort.Slice(matchingPackageMap, func(i, j int) bool {
				return matchingPackageMap[i]["Version"].(string) < matchingPackageMap[j]["Version"].(string)
			})
			for _, versionMap := range matchingPackageMap {
				_parentPackageInformation := ""
				if *showParentPackageInformation {
					_parentPackageInformation = " - Parent/Origin package: " + versionMap["Origin"].(string)
				}
				fmt.Printf("%s (%s - %s) in %s repository%s\n", versionMap["Version"].(string), humanize.Time(versionMap["BuildTime"].(time.Time)), versionMap["BuildTime"].(time.Time), versionMap["Repository"].(string), _parentPackageInformation)
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
			_parentPackageInformation := ""
			if *showParentPackageInformation {
				_parentPackageInformation = " - Parent/Origin package: " + matchingPackageMap["Origin"].(string)
			}
			fmt.Printf("The latest version of package %s is %s (%s - %s) in %s repository%s\n", matchingPackageLatestVersionPackageName, matchingPackageMap["Version"].(string), humanize.Time(matchingPackageMap["BuildTime"].(time.Time)), matchingPackageMap["BuildTime"].(time.Time), matchingPackageMap["Repository"].(string), _parentPackageInformation)
		}

	}
	if *showSubPackageInformation && !*matchAsRegex && len(subPackageNames) > 0 {
		fmt.Println("Sub packages:")
		for _, subPackageName := range subPackageNames {
			fmt.Println(subPackageName)
		}
	}
}
