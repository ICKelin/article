# linux网桥
在[《docker网络之veth设备》](docker网络之veth设备.md)当中提出，veth在多个network namespace之间通信时，需要类似点对点的架构，整个管理会非常复杂，任意两个namespace之间都需要创建veth pair。使用linux网桥可以解决这种困扰。

linux网桥是一种虚拟网络设备，可以类比交换机，允许多个设备连接在其上，也就是交换机的PORT，网桥具备mac地址学习功能。

- 在收到一个数据帧时，记录其源mac地址和对应的PORT的映射关系，进行一轮学习
- 在收到一个包时，检查目的mac地址是否在本地缓存，如果在，则将数据帧转发到具体的PORT，如果不在，则进行泛洪，给除了入PORT之外的所有PORT都拷贝这个帧

那么，将veth桥接到网桥当中，这样通过网桥的自学习和泛洪功能，就可以将数据包从一个namespace发送到另外一个namespace当中。

## 基本使用

- 创建网桥

`brctl addbr br0`
创建br0网桥

- 添加设备到网桥
`brctl addif veth1-0`
`brctl addif veth2-0`

- 查看网桥信息
`brctl show`

## 连通多个namespace

网桥创建成功，添加完设备之后，可以将namespace进行连通，所有操作命令如下:

```
# 创建network namespace
ip netns add netns1
ip netns add netns2

# 创建veth pair
ip link add veth1-0 type veth peer name veth1-1
ip link add veth2-0 type veth peer name veth2-1

ip link set dev veth1-0 up
ip link set dev veth1-1 up
ip link set dev veth2-0 up
ip link set dev veth2-1 up

# 将veth peer加入network namespace
ip link set veth1-1 netns netns1
ip link set veth2-1 netns netns2

ip netns exec netns1 ip link set dev veth1-1 up
ip netns exec netns2 ip link set dev veth2-1 up

# 设置veth pair ip地址
ip netns exec netns1 ip addr add 10.20.30.41/24 dev veth1-1
ip netns exec netns2 ip addr add 10.20.20.41/24 dev veth2-1
ip addr add 10.20.30.40/24 dev veth1-0
ip addr add 10.20.20.40/24 dev veth2-0

# 创建Linux网桥
brctl addbr br0

# 将veth添加到网桥当中
brctl addif br0 veth1-0
brctl addif br0 veth2-0

# 添加默认路由规则
ip netns exec netns1 ip ro add default dev veth1-1
ip netns exec netns2 ip ro add default dev veth2-1
```

上述网络配置完成之后，即可进行连通性测试，在netns1当中，ping netns2的ip

```
ip netns exec netns1 ping 10.20.20.41
ip netns exec netns2 ping 10.20.30.41
```

## 连通host机网络
上述路由配置只能在同一台机器上的不同network namespace进行通信。但是尝试在host机上ping network namespace会出现失败。

```
root@raspberrypi:/home/pi# ping -I veth1-0 10.20.30.41
PING 10.20.30.41 (10.20.30.41) from 10.20.30.40 veth1-0: 56(84) bytes of data.
^C
--- 10.20.30.41 ping statistics ---
2 packets transmitted, 0 received, 100% packet loss, time 1078ms

root@raspberrypi:/home/pi# ping -c 1 -I br0 10.20.30.41
ping: Warning: source address might be selected on device other than br0.
PING 10.20.30.41 (10.20.30.41) from 192.168.31.65 br0: 56(84) bytes of data.
64 bytes from 10.20.30.41: icmp_seq=1 ttl=64 time=0.138 ms

--- 10.20.30.41 ping statistics ---
1 packets transmitted, 1 received, 0% packet loss, time 0ms
rtt min/avg/max/mdev = 0.138/0.138/0.138/0.000 ms

```

如果指定数据包从veth1-0 ping veth1-1的ip则会出现失败，但是从br0上ping则会成功。原因是veth1-0的另外一端已经挂接到br0当中了，在进行二层通信时，首先请求arp，arp响应之后则才能填充mac地址，`ping -I veth1-0 10.20.30.41` 会发送arp请求，veth1-1也响应了arp请求，veth1-0也收到了arp请求。

在host机上抓包
```
rroot@raspberrypi:/home/pi# tcpdump -i veth1-0
tcpdump: verbose output suppressed, use -v or -vv for full protocol decode
listening on veth1-0, link-type EN10MB (Ethernet), capture size 262144 bytes
10:11:02.854145 ARP, Request who-has 10.20.30.41 tell 10.20.30.40, length 28
10:11:02.854207 ARP, Reply 10.20.30.41 is-at 52:68:68:4c:2f:59 (oui Unknown), length 28
10:11:03.923036 ARP, Request who-has 10.20.30.41 tell 10.20.30.40, length 28
10:11:03.923120 ARP, Reply 10.20.30.41 is-at 52:68:68:4c:2f:59 (oui Unknown), length 28
```

有arp请求，也有arp响应，但是响应完成之后，还在源源不断的请求，说明内核还没收到这个arp响应。可以查看arp缓存看下当前arp表信息

```
root@raspberrypi:/home/pi# arp -n
Address                  HWtype  HWaddress           Flags Mask            Iface
10.20.30.41                      (incomplete)                              veth1-0
```

说明内核确实没有收到从veth1-0进来的arp reply，由于veth1-0桥接到br0，arp reply到br0，从br0进入的arp响应很有可能被内核给丢弃掉了。

那么根据这个设想，从br0上`ping -I br0 10.20.30.41`就可以完成了arp学习，mac地址也有了，通信就通了。

```
root@raspberrypi:/home/pi# ping -I br0 10.20.30.41
ping: Warning: source address might be selected on device other than br0.
PING 10.20.30.41 (10.20.30.41) from 192.168.31.65 br0: 56(84) bytes of data.
64 bytes from 10.20.30.41: icmp_seq=1 ttl=64 time=0.137 ms
64 bytes from 10.20.30.41: icmp_seq=2 ttl=64 time=0.132 ms
```

当然这只是表面上解决了与host机通信的问题而已，实际上我们上层在使用的时候不会说指定网卡去发包，而是通过路由帮你选定网卡，为什么`ping 10.20.30.41`会默认从veth1-0网卡发出，路由表就很明确的告诉你，是从veth1-0发出.

```
root@raspberrypi:/home/pi# route -n
Kernel IP routing table
Destination     Gateway         Genmask         Flags Metric Ref    Use Iface
0.0.0.0         192.168.31.1    0.0.0.0         UG    202    0        0 eth0
10.20.20.0      0.0.0.0         255.255.255.0   U     0      0        0 veth2-0
10.20.30.0      0.0.0.0         255.255.255.0   U     0      0        0 veth1-0
```

如果需要`ping 10.20.30.41`走br0出的话，可以将veth1-0的ip地址删了，然后配置一条10.20.30.0/24的路由，网卡为br0

```
# 删除veth1-0的ip地址
ip addr del 10.20.30.40/24 dev veth1-0

# 添加10.20.30.0/24的路由，指定从br0发出
ip ro add 10.20.30.0/24 dev br0

# 在host机上ping 10.20.30.41
root@raspberrypi:/home/pi# ping -c 1 10.20.30.41
PING 10.20.30.41 (10.20.30.41) 56(84) bytes of data.
64 bytes from 10.20.30.41: icmp_seq=1 ttl=64 time=0.157 ms

--- 10.20.30.41 ping statistics ---
1 packets transmitted, 1 received, 0% packet loss, time 0ms
rtt min/avg/max/mdev = 0.157/0.157/0.157/0.000 ms
```

同样，针对netns2也采用同样的操作

## 与外部网络进行通信
上面解决了netns1和netns2的通信，也解决了和host机的通信，但是没有解决和外部网络的通信。在[《docker网络之veth设备》](docker网络之veth设备.md)当中有提到修改路由下一跳的方式，下一跳指定为veth1-0的ip地址，那么现在把veth1-0的ip地址删了，下一跳没法填了。这个问题需要解决。

可以尝试给br0设置一个地址，让veth1-0的下一跳ip指向br0的ip。

- 给br0一个ip地址
`ip addr add 10.20.30.1/24 dev br0`

- 修改netns1的路由
```
ns1> ip ro del default dev veth1-1
ns1> route add default gw 10.20.30.1
ns1> ping 114.114.114.114
PING 114.114.114.114 (114.114.114.114) 56(84) bytes of data.
64 bytes from 114.114.114.114: icmp_seq=1 ttl=83 time=33.8 ms
64 bytes from 114.114.114.114: icmp_seq=2 ttl=63 time=34.0 ms
```

通过这种方式修改之后，netns1就可以和外界进行通信了。但是另外一个问题又来了，netns2怎么办，那尝试给br0再添加一个地址？

```
ip addr add 10.20.20.1/24 dev br0

ns2> ip ro del default dev veth2-1
ns2> route add default gw 10.20.20.1
ns2> ping 114.114.114.114
PING 114.114.114.114 (114.114.114.114) 56(84) bytes of data.
64 bytes from 114.114.114.114: icmp_seq=1 ttl=67 time=34.3 ms
```

确实是可以这么做。但是可以尝试使用另外一种方法，没有人规定netns1和netns2一定不在同一个子网，只需要划分一个大的子网，给br0配一个ip地址就行，这样就不会导致netns1和netns2需要添加不同的路由。

## 总结
docker网络基础的三篇文章

- [docker网络之namespace](docker网络之namespace.md)
- [docker网络之veth设备](docker网络之veth设备.md)
- [docker网络之网桥](docker网络之网桥.md)

已全部写完，这三项技术我认为是网络的基础只是，也是docker网络的基础，解决最基本的容器通信问题，只有解决容器与内部，外部网络的通信问题，才能在其之上拓展，比如说：

1. 假设需要在外部访问netns1中的一个web服务，只有host机能与netns1通信了，我们在可以利用host机这个跳板跳进netns1
2. 假设netns1需要访问外网服务，比如说域名解析，同样需要依赖host机作为跳板跳出去.

接下来会写一些真真正正跟docker网络相关文章，但是主要还是以网络为主，docker为辅助，说明这些网络技术是如何在docker当中实践的。

