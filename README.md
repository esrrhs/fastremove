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

**fastremove** walks a directory tree, hashes each file with MD5, and reports duplicates. With `-delete`, it keeps the first copy and removes the rest.

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

Scan the current directory for duplicates (dry run — no deletion):

```bash
./fastremove -path ./ -method duplicate
```

Delete duplicate files automatically:

```bash
./fastremove -path ./ -method duplicate -delete
```

Show version:

```bash
./fastremove -version
```

More flags:

```text
Usage of ./fastremove:
  -delete
        auto delete
  -method string
        delete method: duplicate (default "duplicate")
  -path string
        scan path (default "./")
  -thread int
        replace thread (default 32)
  -v    show info
  -version
        show version
```

## Build release packages

```bash
./pack.sh
```

Cross-platform archives are written to `pack/`. The **Release** workflow watches `version.go`: when its version string changes on `master` (and the tag does not exist yet), CI runs `./pack.sh` and publishes a GitHub Release.

## License

MIT — see [LICENSE](LICENSE).
