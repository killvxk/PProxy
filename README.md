## 简介

根据windivert+tun2socks+shadow完成的进程加速程序，写这个主要是为了打游戏的。

## 配置文件

- 代理服务配置文件
```json
{
  "type": "socks5",//目前只支持socks5和shadowsocks
  "server": "127.0.0.1",
  "server_port": 1080,
  "method": "",
  "password": "",
  "plugin": "",
  "plugin_opts": ""
}
```
- 进程配置文件
```json
{
  "processes": [
    "chrome.exe"
  ],
  "whitelist": [
    "www.google.com"
  ]
}
```
- 运行`PProxy.exe -sconfig server.json -pconfig process.json`, 需要管理员权限

## 感谢以下大佬的项目(基本上的代码都来自以下项目)

- https://github.com/eycorsican/go-tun2socks
- https://github.com/imgk/shadow
- https://github.com/basil00/Divert