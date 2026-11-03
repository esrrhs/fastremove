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

**fastremove** 递归遍历目录，用 MD5 哈希识别重复文件，并报告每组中保留哪个、删除哪些。**默认仅为 dry-run（只报告，不删除）**；加上 `-delete` 才会真正删除。可用 `-keep` 选择保留策略。

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

将报告写入文件，再删除非保留项：

```bash
./fastremove -path ./ -method duplicate -report dups.txt
./fastremove -path ./ -method duplicate -keep newest -delete
```

查看版本：

```bash
./fastremove -version
```

更多参数：

```text
Usage of ./fastremove:
  -delete
        真正删除重复文件（默认仅 dry-run 报告）
  -keep string
        保留策略：first、newest、shortest-path（默认 "first"）
  -method string
        删除方法：duplicate（默认 "duplicate"）
  -path string
        扫描路径（默认 "./"）
  -report string
        可选，将重复报告写入该路径（默认 stdout）
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

## 打包发布

```bash
./pack.sh
```

跨平台压缩包输出到 `pack/`。**Release** 流水线会监视 `version.go`：当其版本号在 `master` 上发生变更（且对应 tag 尚不存在）时，CI 会执行 `./pack.sh` 并发布 GitHub Release。

## 许可

MIT — 见 [LICENSE](LICENSE)。
