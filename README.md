## PProxy

根据windivert+tun2socks+shadow完成的进程加速程序，主要用于外服游戏加速。

## 需求

1. 使用windivert拦截指定进程的数据包，包括dns数据
2. 将捕获的数据包通过代理发送出去

## 项目设计

- PProxy
  - proxy
  - utils
  - main.go