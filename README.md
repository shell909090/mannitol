# dns代理

没有缓存，没有多余功能。仅将每个dns请求加一个edns client subnet。主要解决向一个远程DNS服务器做请求导致的位置偏移问题。

一般用法是这样。前面是dnsmasq缓存，后面是防污染方案（例如VPN），上游需要支持edns client subnet。因此默认是在路由器上。

# 鸣谢

ayanamist的[gdns-go](https://github.com/ayanamist/gdns-go)
