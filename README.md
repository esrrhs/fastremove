# fastremove

[<img src="https://img.shields.io/github/license/esrrhs/fastremove">](https://github.com/esrrhs/fastremove)
[<img src="https://img.shields.io/github/languages/top/esrrhs/fastremove">](https://github.com/esrrhs/fastremove)
[![Go Report Card](https://goreportcard.com/badge/github.com/esrrhs/fastremove)](https://goreportcard.com/report/github.com/esrrhs/fastremove)
[<img src="https://img.shields.io/github/v/release/esrrhs/fastremove">](https://github.com/esrrhs/fastremove/releases)
[<img src="https://img.shields.io/github/downloads/esrrhs/fastremove/total">](https://github.com/esrrhs/fastremove/releases)
[<img src="https://img.shields.io/github/actions/workflow/status/esrrhs/fastremove/go.yml?branch=master">](https://github.com/esrrhs/fastremove/actions)

多线程删除重复文件

# 使用
* 遍历当前目录及子目录的所有文件，根据MD5移除重复文件
```
# ./fastremove -path ./ -method duplicate -delete
```
* 更多参数参考-h
```
Usage of fastremove.exe:
  -delete
        auto delete
  -method string
        delete method: duplicate (default "duplicate")
  -path string
        scan path (default "./")
  -thread int
        replace thread (default 32)
  -v    show info
```
