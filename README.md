# fastremove

[<img src="https://img.shields.io/github/license/esrrhs/fastremove">](https://github.com/esrrhs/fastremove)
[<img src="https://img.shields.io/github/languages/top/esrrhs/fastremove">](https://github.com/esrrhs/fastremove)
[<img src="https://img.shields.io/github/v/release/esrrhs/fastremove">](https://github.com/esrrhs/fastremove/releases)
[<img src="https://img.shields.io/github/downloads/esrrhs/fastremove/total">](https://github.com/esrrhs/fastremove/releases)
[<img src="https://img.shields.io/github/actions/workflow/status/esrrhs/fastremove/go.yml?branch=master">](https://github.com/esrrhs/fastremove/actions)

> **A fast multithreaded CLI tool that finds and removes duplicate files by MD5.**

[English](README.md) | [Chinese](README_CN.md)

---

## Overview

**fastremove** walks a directory tree, hashes each file with MD5, and reports duplicate groups (which file would be kept vs deleted). **By default it is dry-run only** — nothing is removed unless you pass `-delete`. Use `-keep` to choose which copy survives.

## Install

Download a prebuilt binary from the **[Releases](https://github.com/esrrhs/fastremove/releases)** page, or build from source:

```bash
git clone https://github.com/esrrhs/fastremove.git
cd fastremove
go build -o fastremove .
```

Docker:

```bash
docker build -t fastremove .
```

## Usage

Scan the current directory for duplicates (dry-run report — no deletion):

```bash
./fastremove -path ./ -method duplicate
```

Choose which file to keep in each group:

```bash
./fastremove -path ./ -method duplicate -keep newest
./fastremove -path ./ -method duplicate -keep shortest-path
```

Write the report to a file, then delete non-keepers:

```bash
./fastremove -path ./ -method duplicate -report dups.txt
./fastremove -path ./ -method duplicate -keep newest -delete
```

Show version:

```bash
./fastremove -version
```

More flags:

```text
Usage of ./fastremove:
  -delete
        actually delete duplicate files (default: dry-run report only)
  -keep string
        keep strategy: first, newest, shortest-path (default "first")
  -method string
        delete method: duplicate (default "duplicate")
  -path string
        scan path (default "./")
  -report string
        optional path to write duplicate report (default: stdout)
  -thread int
        hash worker concurrency (default 32)
  -v    show info
  -version
        show version
```

Keep strategies:

- `first` (default): keep the first file seen in walk order
- `newest`: keep the file with the newest modification time
- `shortest-path`: keep the path with the fewest separators (then shorter string, then lexical)

## Build release packages

```bash
./pack.sh
```

Cross-platform archives are written to `pack/`. The **Release** workflow watches `version.go`: when its version string changes on `master` (and the tag does not exist yet), CI runs `./pack.sh` and publishes a GitHub Release.

## License

MIT — see [LICENSE](LICENSE).
