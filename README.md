# wolfi-package-status
This is a simple CLI tool that lists the latest version of a given package across all wolfi repositories.

Currently only supports querying the x86_64/amd64 repostiries.

The repositories queried are:

-   Wolfi OS @ [https://packages.wolfi.dev/os/x86_64/APKINDEX.tar.gz](https://packages.wolfi.dev/os/x86_64/APKINDEX.tar.gz)
-   Wolfi OS Enterprise Packages (Non Free maintained by Chainguard) @ [https://packages.cgr.dev/os/x86_64/APKINDEX.tar.gz](https://packages.cgr.dev/os/x86_64/APKINDEX.tar.gz)
-   Wolfi OS Extra Packages (Non Free maintained by Chainguard) @ [https://packages.cgr.dev/extras/x86_64/APKINDEX.tar.g](https://packages.cgr.dev/extras/x86_64/APKINDEX.tar.gz)

## installation 

```bash
go get github.com/philroche/wolfi-package-status
```
## usage

Display usage instructions and help text
```bash
wolfi-package-status --help
```

List latest version of a known package name
```bash
wolfi-package-status python-3.12
```

List latest version of a multiple known package names
```bash
wolfi-package-status python-3.11 python-3.12
```

List latest versions of packages with package name matching regex python-3.12.*
```bash
wolfi-package-status --regex "python-3.12.*"
```

List latest versions of packages with package names matching regexes python-3.11.* and python-3.12.*
```bash
wolfi-package-status --regex "python-3.11.*" "python-3.12.*"
```


List all packages and versions across all Wolfi repostories
```bash
wolfi-package-status
```