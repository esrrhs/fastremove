# fastremove

[<img src="https://img.shields.io/github/license/esrrhs/fastremove">](https://github.com/esrrhs/fastremove)
[<img src="https://img.shields.io/github/languages/top/esrrhs/fastremove">](https://github.com/esrrhs/fastremove)
[<img src="https://img.shields.io/github/v/release/esrrhs/fastremove">](https://github.com/esrrhs/fastremove/releases)
[<img src="https://img.shields.io/github/downloads/esrrhs/fastremove/total">](https://github.com/esrrhs/fastremove/releases)
[<img src="https://img.shields.io/github/actions/workflow/status/esrrhs/fastremove/go.yml?branch=master">](https://github.com/esrrhs/fastremove/actions)

> **多线程按 MD5 查找并删除重复文件的命令行工具。**

[English](README.md) | [中文说明](README_CN.md)

---

## 简介

**fastremove** 递归遍历目录，用 MD5 识别重复文件，并报告每组中保留哪个、移除哪些。**默认仅为 dry-run（只报告，不删除）**；加上 `-delete` 才会处理。可用 `-keep` 选择保留策略。

`-delete` 是可恢复的：在操作系统回收站/废纸篓实际可用时优先放入其中，否则移入带持久清单（manifest）的隔离目录（quarantine）。不会静默永久删除，也没有默认的永久删除开关。

每次移除前都会复核保留项与候选文件（必须是普通文件、大小/修改时间与扫描快照一致、身份未被替换，并且当前内容逐字节相同，而不仅是 MD5 相同）。扫描不跟随符号链接。硬链接会被保守跳过，不会解除链接。

## 安装

从 **[Releases](https://github.com/esrrhs/fastremove/releases)** 下载预编译二进制，或自行编译：

```bash
git clone https://github.com/esrrhs/fastremove.git
cd fastremove
go build -o fastremove .
```

Docker：

```bash
docker build -t fastremove .
```

## 使用

扫描当前目录下的重复文件（仅报告，不删除）：

```bash
./fastremove -path ./ -method duplicate
```

选择每组保留哪一份：

```bash
./fastremove -path ./ -method duplicate -keep newest
./fastremove -path ./ -method duplicate -keep shortest-path
```

将报告写入文件，再可恢复地移除非保留项：

```bash
./fastremove -path ./ -method duplicate -report dups.txt
./fastremove -path ./ -method duplicate -keep newest -delete
```

从某次隔离会话的清单恢复文件（已存在的目标路径不会被覆盖）：

```bash
./fastremove -restore ~/.cache/fastremove/quarantine/<session>/manifest.json
```

查看版本：

```bash
./fastremove -version
```

参数：

```text
  -delete
        可恢复地移除重复文件（回收站可用则进回收站，否则隔离）；
        默认仅 dry-run 报告
  -keep string
        保留策略：first、newest、shortest-path（默认 "first"）
  -method string
        删除方法：duplicate（默认 "duplicate"）
  -path string
        扫描路径（默认 "./"）
  -quarantine-dir string
        回收站不可用时的隔离根目录（默认：用户缓存目录）
  -report string
        可选，将报告写入该路径（默认 stdout；不会覆盖已扫描的原文件或保留项）
  -restore string
        从隔离目录的 manifest.json 恢复文件
  -thread int
        哈希并发数（默认 32）
  -v    显示详细信息
  -version
        显示版本
```

保留策略：

- `first`（默认）：按遍历顺序保留最先看到的文件
- `newest`：保留修改时间最新的文件
- `shortest-path`：保留路径分隔符最少的（其次更短字符串，再按字典序）

## 移除流程

1. 只扫描普通文件。符号链接、设备文件和目录会被跳过；不会穿越目录符号链接。隔离目录（包括 `.fastremove-quarantine` 以及带有 fastremove 隔离标记的目录树）不会再次被扫描。
2. 先按文件大小分组。仅当某一大小下至少有两个不同文件（在能取得身份信息时，指不同的设备号+inode）时才计算哈希。大小唯一的文件不会被哈希。
3. 使用 `-delete` 时，在每次操作前立即复核保留项与候选：仍为普通文件、大小和修改时间与扫描快照一致、身份未被替换、不是同一 inode、候选 `nlink == 1`，且当前内容逐字节相同。任一不匹配都会跳过并报告，文件留在原地。
4. 随后将候选移入操作系统回收站（若该后端成功），否则移入隔离目录。跨设备移动会被拒绝（不会先复制再删除）。若移动后无法写入隔离清单，会尝试将文件改回原路径并报告失败。

### 各平台回收站与回退

| 平台 | 回收站后端 | 回退 |
| --- | --- | --- |
| Linux | Freedesktop XDG 回收站（`$XDG_DATA_HOME/Trash` 或 `~/.local/share/Trash`）。直接实现，不经过 shell。**已在当前开发环境中验证**（含隔离的测试目录）。 | 回收站不可用或移动会跨设备时，改走隔离目录。 |
| macOS | 在存在 `osascript` 时，通过带引号的 POSIX 路径调用 Finder（无 shell 拼接）。**此处未实际执行或验证**（无 macOS 环境）。 | 若缺少 `osascript`/Finder 或命令失败，改走隔离目录。 |
| Windows | 通过 `SHFileOperationW` + `FOF_ALLOWUNDO` 使用回收站。**此处未实际执行或验证**（无 Windows 环境）。 | API 失败时改走隔离目录。 |
| 其他 | 无。 | 仅隔离目录。 |

测试或特殊部署可用 `FASTREMOVE_TRASH_HOME`（Linux 回收站父目录）以及 `FASTREMOVE_QUARANTINE` 或 `-quarantine-dir` 覆盖位置。默认隔离根目录为 `<用户缓存>/fastremove/quarantine/<session>/`。

恢复方式：

- **进入回收站** 的文件，请用系统的回收站 / Recycle Bin / Finder 废纸篓界面恢复。
- **进入隔离目录** 的文件，使用 `./fastremove -restore <manifest.json>`。恢复会拒绝相对路径、包含 `..` 的路径、符号链接祖先目录，以及已存在的目标（不覆盖）。若恢复将跨设备，会直接拒绝而不会复制。

### 限制（请阅读）

- 复核与移除不是单一原子操作。在最后一次复核之后、rename/移入回收站之前的短暂窗口内，文件仍可能被替换。本工具**不保证**在任何平台上都是无竞态删除。Windows 上，若该窗口内出现重解析点，`Open` 仍可能跟随它。
- 若 inode 被复用，且大小、修改时间和字节内容都相同，理论上可能掩盖替换。
- 硬链接（`nlink > 1`，或与保留项同一 inode）会被跳过，不会解除链接。
- 跨设备的回收站或隔离会被拒绝；如有需要，请把 `-quarantine-dir` 设在与文件相同的文件系统上。
- `-report` 不会覆盖已扫描的原文件或保留项，但可以覆盖该路径上的旧报告。

## 打包发布

```bash
./pack.sh
```

跨平台压缩包输出到 `pack/`。**Release** 流水线会监视 `version.go`：当其版本号在 `master` 上发生变更（且对应 tag 尚不存在）时，CI 会执行 `./pack.sh` 并发布 GitHub Release。

## 许可

MIT — 见 [LICENSE](LICENSE)。
