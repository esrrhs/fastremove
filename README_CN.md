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

**fastremove** 递归遍历目录，用 MD5 哈希识别重复文件并报告。加上 `-delete` 时，保留首次出现的副本，删除其余重复项。

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

自动删除重复文件：

```bash
./fastremove -path ./ -method duplicate -delete
```

查看版本：

```bash
./fastremove -version
```

更多参数：

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

## 打包发布

```bash
./pack.sh
```

跨平台压缩包输出到 `pack/`。**Release** 流水线会监视 `version.go`：当其版本号在 `master` 上发生变更（且对应 tag 尚不存在）时，CI 会执行 `./pack.sh` 并发布 GitHub Release。

## 许可

MIT — 见 [LICENSE](LICENSE)。
