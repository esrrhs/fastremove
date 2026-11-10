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

**fastremove** walks a directory tree, hashes files with MD5, and reports duplicate groups (which file would be kept vs removed). **By default it is dry-run only** — nothing is removed unless you pass `-delete`. Use `-keep` to choose which copy survives.

`-delete` is recoverable: the tool prefers the OS trash/recycle bin when that backend is actually available, and otherwise moves files into a quarantine directory with a durable manifest. There is no silent permanent delete and no default permanent-delete flag.

Before each removal it revalidates the keeper and the candidate (regular file, size/mtime vs the scan snapshot, identity, and byte-for-byte content — not just MD5). Symlinks are not followed during the walk. Hard links are treated conservatively and are not removed.

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

Write the report to a file, then recoverably remove non-keepers:

```bash
./fastremove -path ./ -method duplicate -report dups.txt
./fastremove -path ./ -method duplicate -keep newest -delete
```

Restore quarantined files from a session manifest (never overwrites an existing path):

```bash
./fastremove -restore ~/.cache/fastremove/quarantine/<session>/manifest.json
```

Show version:

```bash
./fastremove -version
```

Flags:

```text
  -delete
        recoverably remove duplicates (trash if available, else quarantine);
        default is dry-run report only
  -keep string
        keep strategy: first, newest, shortest-path (default "first")
  -method string
        delete method: duplicate (default "duplicate")
  -path string
        scan path (default "./")
  -quarantine-dir string
        quarantine root used when trash is unavailable (default: user cache)
  -report string
        optional path to write the report (default: stdout; will not overwrite
        a scanned original or keeper)
  -restore string
        restore files from a quarantine manifest.json
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

## How removal works

1. Only regular files are scanned. Symlinks, devices, and directories are skipped. Directory symlinks are not traversed. Quarantine directories (including `.fastremove-quarantine` and any tree marked as a fastremove quarantine root) are excluded from later scans.
2. Files are first grouped by size. Only size groups with at least two distinct files (distinct device+inode when identity is available) are hashed. Unique-size files are never hashed.
3. With `-delete`, immediately before each action the tool rechecks keeper and candidate: still regular files, size and mtime unchanged from the scan snapshot, identity not replaced, not the same inode, candidate `nlink == 1`, and current contents are byte-for-byte equal. Mismatches are skipped and reported; the file is left in place.
4. The candidate is then moved to the OS trash when that backend succeeds, otherwise into quarantine. Cross-device moves are refused (no copy-then-delete). If the quarantine journal/manifest cannot be written after a move, the tool tries to rename the file back and reports failure.

### Platform trash / fallback

| Platform | Trash backend | Fallback |
| --- | --- | --- |
| Linux | Freedesktop XDG trash (`$XDG_DATA_HOME/Trash` or `~/.local/share/Trash`). Implemented directly, not via a shell. **Verified in this development environment** (including isolated test roots). | If trash is unavailable or the move would cross devices, files go to quarantine. |
| macOS | Finder via `osascript` with a quoted POSIX path (no shell interpolation), when `osascript` is present. **Not executed or verified here** (no macOS runner). | Quarantine if `osascript`/Finder is missing or the command fails. |
| Windows | Recycle Bin via `SHFileOperationW` + `FOF_ALLOWUNDO`. **Not executed or verified here** (no Windows runner). | Quarantine if the API fails. |
| Other | None. | Quarantine only. |

Override trash or quarantine locations in tests or special setups with `FASTREMOVE_TRASH_HOME` (Linux trash root parent) and `FASTREMOVE_QUARANTINE` or `-quarantine-dir`. The default quarantine root is `<user-cache>/fastremove/quarantine/<session>/`.

Recovery:

- **Trashed** files are restored with the OS trash / Recycle Bin / Finder Trash UI.
- **Quarantined** files are restored with `./fastremove -restore <manifest.json>`. Restore refuses relative paths, `..` path components, symlink ancestors, and existing destinations (no overwrite). If a restore would cross devices, it is refused rather than copied.

### Limitations (please read)

- Recheck and remove are not a single atomic operation. A file can still be replaced in the short window after the last revalidation and before the rename/trash call. This tool does **not** promise race-free deletion on any platform. On Windows, `Open` can also follow a reparse point that appears in that window.
- Inode reuse plus identical size, mtime, and bytes can theoretically hide a replacement.
- Hard-linked names (`nlink > 1`, or the same inode as the keeper) are skipped, not unlinked.
- Cross-device trash or quarantine is refused; set `-quarantine-dir` on the same filesystem as the files if needed.
- `-report` will not overwrite a scanned original or keeper, but it may overwrite a previous report at that path.

## Build release packages

```bash
./pack.sh
```

Cross-platform archives are written to `pack/`. The **Release** workflow watches `version.go`: when its version string changes on `master` (and the tag does not exist yet), CI runs `./pack.sh` and publishes a GitHub Release.

## License

MIT — see [LICENSE](LICENSE).
