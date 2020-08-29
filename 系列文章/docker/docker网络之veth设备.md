# veth设备
在之前[《docker网络之namespace》](docker网络之namespace.md)这篇文章最后提到network namespace无法与外界进行网络互通的问题，而veth设备可以解决namespace之间网络联通的问题。

## veth设备原理
linux虚拟网络设备有很多，而且都非常有用，比如说tun/tap设备主要用于构建VPN的，veth设备对解决namespace通信问题，还有比如说虚拟网桥和vxlan相关的设备等。后续会有相关文章详细介绍Linux网桥（bridge）和tun/tap设备，这些设备在docker网络当中扮演着非常重要的角色。

正常的网卡是连接物理网络和内核协议栈的，也就是工作在二层，位于网络协议栈L3和物理链路L1之间，veth设备则不然，veth设备是成对出现的，所以通常也成为veth设备对或者veth pair。

veth全称是(Virtual Ethernet)虚拟以太网，其原理类似linux管道，在一个veth设备写入网络包，其对端的veth设备可以读取到对应的网络包。

```
图片来源: https://segmentfault.com/a/1190000009251098

+----------------------------------------------------------------+
|                                                                |
|       +------------------------------------------------+       |
|       |             Newwork Protocol Stack             |       |
|       +------------------------------------------------+       |
|              ↑               ↑               ↑                 |
|..............|...............|...............|.................|
|              ↓               ↓               ↓                 |
|        +----------+    +-----------+   +-----------+           |
|        |   eth0   |    |   veth0   |   |   veth1   |           |
|        +----------+    +-----------+   +-----------+           |
|192.168.1.11  ↑               ↑               ↑                 |
|              |               +---------------+                 |
|              |         192.168.2.11     192.168.2.1            |
+--------------|-------------------------------------------------+
               ↓
         Physical Network
```

## 基本操作

- 创建veth pair

`ip link add veth1-0 type veth peer name veth1-1`
创建成功之后，使用ifconfig命令可以查看到网卡信息

```
veth1-0: flags=4163<UP,BROADCAST,RUNNING,MULTICAST>  mtu 1500
        inet 169.254.130.158  netmask 255.255.0.0  broadcast 169.254.255.255
        inet6 fe80::6715:981b:b598:bb4  prefixlen 64  scopeid 0x20<link>
        ether 66:1e:33:6f:c3:17  txqueuelen 1000  (Ethernet)
        RX packets 34  bytes 5538 (5.4 KiB)
        RX errors 0  dropped 0  overruns 0  frame 0
        TX packets 34  bytes 5538 (5.4 KiB)
        TX errors 0  dropped 0 overruns 0  carrier 0  collisions 0

root@raspberrypi:/home/pi# ifconfig veth1-1
veth1-1: flags=4163<UP,BROADCAST,RUNNING,MULTICAST>  mtu 1500
        inet 169.254.217.246  netmask 255.255.0.0  broadcast 169.254.255.255
        inet6 fe80::7b5b:c1b9:e0ba:e75b  prefixlen 64  scopeid 0x20<link>
        ether ee:b6:7b:94:55:26  txqueuelen 1000  (Ethernet)
        RX packets 34  bytes 5538 (5.4 KiB)
        RX errors 0  dropped 0  overruns 0  frame 0
        TX packets 34  bytes 5538 (5.4 KiB)
        TX errors 0  dropped 0 overruns 0  carrier 0  collisions 0

```
将两张网卡UP起来

`ip link set dev veth1-0 up`
`ip link set dev veth1-1 up`

执行完上述操作之后，就可以像配置真实网卡一样配置veth设备类型的网卡。

## 利用veth连接多个network namespace

首先host机本身就有一个network namespace，在《docker网络之namespace》中创建了一个netns1，可以使用veth设备将两个namespace网络打通。

首先将veth1-1放到netns1当中
`ip link set veth1-1 netns netns1`

然后切换至netns1，并查看网卡信息，并将veth1-1 UP起来

```
root@raspberrypi:/home/pi# ip netns exec netns1 bash --rcfile <(echo "PS1=\"ns1> \"")
ns1> ip link list
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN mode DEFAULT group default qlen 1000
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
419: veth1-1@if420: <BROADCAST,MULTICAST> mtu 1500 qdisc noop state DOWN mode DEFAULT group default qlen 1000
    link/ether ee:b6:7b:94:55:26 brd ff:ff:ff:ff:ff:ff link-netnsid 0
ns1> ip link set dev veth1-1 up
ns1> ifconfig
lo: flags=73<UP,LOOPBACK,RUNNING>  mtu 65536
        inet 127.0.0.1  netmask 255.0.0.0
        inet6 ::1  prefixlen 128  scopeid 0x10<host>
        loop  txqueuelen 1000  (Local Loopback)
        RX packets 0  bytes 0 (0.0 B)
        RX errors 0  dropped 0  overruns 0  frame 0
        TX packets 0  bytes 0 (0.0 B)
        TX errors 0  dropped 0 overruns 0  carrier 0  collisions 0

veth1-1: flags=4163<UP,BROADCAST,RUNNING,MULTICAST>  mtu 1500
        inet6 fe80::ecb6:7bff:fe94:5526  prefixlen 64  scopeid 0x20<link>
        ether ee:b6:7b:94:55:26  txqueuelen 1000  (Ethernet)
        RX packets 54  bytes 10010 (9.7 KiB)
        RX errors 0  dropped 0  overruns 0  frame 0
        TX packets 45  bytes 7979 (7.7 KiB)
        TX errors 0  dropped 0 overruns 0  carrier 0  collisions 0

ns1>
```

网卡配置完毕之后，接下来需要配置ip地址，因为没有ip地址就发不了包，发包的ip地址就变成0.0.0.0

- 在netns1当中配置veth1-1的ip地址
`ns1> ip addr add 10.20.30.41/24 dev veth1-1`

- 在host配置veth1-0的ip地址
`ip addr add 10.20.30.40/24 dev veth1-0`

通过这项配置之后，无论在netns1当中ping host机ip还是ping veth1-0的ip，都是没问题的
```
ns1> ping -I veth1-1 192.168.31.65
PING 192.168.31.65 (192.168.31.65) from 10.20.30.41 veth1-1: 56(84) bytes of data.
64 bytes from 192.168.31.65: icmp_seq=1 ttl=64 time=0.169 ms
^C
--- 192.168.31.65 ping statistics ---
1 packets transmitted, 1 received, 0% packet loss, time 0ms
rtt min/avg/max/mdev = 0.169/0.169/0.169/0.000 ms
ns1> ping 10.20.30.40
PING 10.20.30.40 (10.20.30.40) 56(84) bytes of data.
64 bytes from 10.20.30.40: icmp_seq=1 ttl=64 time=0.197 ms
```

但是需要注意的是在ping host机的ip时，需要指定网卡，根本原因是没有默认路由，host机ip和veth pair不在同一个网段，所以在ping host_ip的时候就会找不到路由。尝试添加默认路由解决这个问题。

```
ns1> route add default gw 10.20.30.40
ns1> ping 192.168.31.65
PING 192.168.31.65 (192.168.31.65) 56(84) bytes of data.
64 bytes from 192.168.31.65: icmp_seq=1 ttl=64 time=0.149 ms
^C
--- 192.168.31.65 ping statistics ---
1 packets transmitted, 1 received, 0% packet loss, time 0ms
rtt min/avg/max/mdev = 0.149/0.149/0.149/0.000 ms
ns1> ping 114.114.114.114
PING 114.114.114.114 (114.114.114.114) 56(84) bytes of data.
64 bytes from 114.114.114.114: icmp_seq=1 ttl=87 time=33.8 ms
^C
--- 114.114.114.114 ping statistics ---
1 packets transmitted, 1 received, 0% packet loss, time 0ms
rtt min/avg/max/mdev = 33.861/33.861/33.861/0.000 ms
```

默认路由指定下一跳的是veth1-0，这种方式无论是ping host_ip还是ping公网ip，都可以ping得通，如果`ip ro add default dev veth1-1`的方式添加，ping 公网ip将无法ping通，会发现一直在发arp请求，请求114.114.114.114的mac地址。原因应该是没有下一跳地址。

```
ns1> route -n
Kernel IP routing table
Destination     Gateway         Genmask         Flags Metric Ref    Use Iface
0.0.0.0         0.0.0.0         0.0.0.0         U     0      0        0 veth1-1
10.20.30.0      0.0.0.0         255.255.255.0   U     0      0        0 veth1-1
```

第一条路由就说明了，任何地址都在一个子网，在一个子网，当然发ARP查询mac地址进行二层通信了。

## veth pair的缺点
veth pair相当于网线连接两个网口，打个比喻，我们平时使用电脑插路由器网线，在你电脑的网口和路由器的lan口就是veth pair。
其不足也在这里，只能连接两个network namespace，如果要多个network namespace进行通信，会非常复杂，你会建立一系列的veth pair，整个关系网是点对点的，也就是任意两个network namespace都需要veth pair来通信。

这个问题的解决办法需要依赖linux网桥(bridge)，利用网桥来将多个veth设备连接起来，这部分将在[docker网络之网桥](docker网络之网桥.md)进行阐述。

