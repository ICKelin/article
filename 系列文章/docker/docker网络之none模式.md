# none模式
之前写了三篇网络相关的技术

- [docker网络之namespace](docker网络之namespace.md)
- [docker网络之veth设备](docker网络之veth设备.md)
- [docker网络之网桥](docker网络之网桥.md)

接下来考虑将这些技术用在docker当中，docker网络包含四种网络模式，分别是none,container,bridge,host。

其中none模式下的容器之后lo网卡，无法联网，但是优点的可操作性最强，也就是说发挥的地方更多。

本文将创建一个none模式的容器，然后通过命令创建veth pair，再通过路由配置实现none模式的容器联网。

## 基本操作

- 准备容器
```
docker run -d --network=none --restart=always nginx
docker inspect sharp_khorana|grep Pid
```

ip netns命令只能操作/var/run/netns/ 目录下的network namespace，docker创建的namespace不在这上面，需要建立软连接
`ln -s /proc/14915/ns/net /var/run/netns/netns_nginx`
成功之后就可以进入netns_nginx这个namespace了。

```
# 创建veth pair
ip link add veth3-0 type veth peer name veth3-1

ip link set veth3-0 up
ip link set veth3-1 up

# 将veth3-0桥接到docker 网桥当中
brctl addif docker0 veth3-0

# 将veth3-1加入netns_nginx当中
link set veth3-1 netns netns_nginx
ip netns exec netns_nginx ip link set veth3-1 up

# 配置ip
ip netns exec netns_nginx ip addr add 172.21.0.199/16 dev veth3-1

# 添加路由
ip netns exec netns_nginx ip ro del default dev veth3-1
ip netns exec netns_nginx route add default gw 172.21.0.1
```

上述配置完成了创建veth pair，将其中一端加入netns_nginx这个namespace当中，将另外一端桥接到docker0，并且给namespace添加默认路由，下一跳指向docker0的ip地址。

完成以上配置之后，一个none模式的容器就具备与外网通信的功能了。

## 尝试在容器内部访问外网
```
root@raspberrypi:/home/pi# ip netns exec netns_nginx bash --rcfile <(echo "PS1=\"nginx> \"")
nginx> nslookup www.notr.tech 114.114.114.114
Server:		114.114.114.114
Address:	114.114.114.114#53

Non-authoritative answer:
Name:	www.notr.tech
Address: 47.115.119.45

nginx>

```

## 尝试访问容器内部
由于没有配置任何端口转发，所以没法在外部通过host机访问内部。

先测试在host机访问内部
```
root@raspberrypi:/home/pi# curl 172.21.0.199
<!DOCTYPE html>
<html>
<head>
<title>Welcome to nginx!</title>
<style>
    body {
        width: 35em;
        margin: 0 auto;
        font-family: Tahoma, Verdana, Arial, sans-serif;
    }
</style>
</head>
<body>
<h1>Welcome to nginx!</h1>
<p>If you see this page, the nginx web server is successfully installed and
working. Further configuration is required.</p>

<p>For online documentation and support please refer to
<a href="http://nginx.org/">nginx.org</a>.<br/>
Commercial support is available at
<a href="http://nginx.com/">nginx.com</a>.</p>

<p><em>Thank you for using nginx.</em></p>
</body>
</html>
root@raspberrypi:/home/pi#

```

为了在外部通过host访问，可以添加一条iptables的DNAT规则，将host机的9097端口映射到容器172.21.0.199:80端口
`iptables -t nat -I PREROUTING -p tcp --dport 9097 -j DNAT --to 172.21.0.199:80`

```
➜  ICKelin curl http://192.168.31.65:9097
<!DOCTYPE html>
<html>
<head>
<title>Welcome to nginx!</title>
<style>
    body {
        width: 35em;
        margin: 0 auto;
        font-family: Tahoma, Verdana, Arial, sans-serif;
    }
</style>
</head>
<body>
<h1>Welcome to nginx!</h1>
<p>If you see this page, the nginx web server is successfully installed and
working. Further configuration is required.</p>

<p>For online documentation and support please refer to
<a href="http://nginx.org/">nginx.org</a>.<br/>
Commercial support is available at
<a href="http://nginx.com/">nginx.com</a>.</p>

<p><em>Thank you for using nginx.</em></p>
</body>
</html>
➜  ICKelin 
```
这个配置也就是docker bridge模式的雏形，bridge本质上也是通过host机的端口映射到容器的端口，所有容器共享宿主机的端口信息。

## 总结
docker 的none模式给了技术任意更多的控制力，是一张白纸，可以让工程师绘制出更加美好的蓝图。但是我们应该站在巨人的肩膀上，尽量减少重复劳动，所以如果可以，尽量使用docker的bridge和host模式。