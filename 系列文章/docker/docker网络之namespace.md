# network namespace
network namespace主要是用来进行网络隔离的，将网络划分为不同的namespace，不同namespace的操作互不影响，每个network namespace包含:

1. 网卡和mac地址
2. arp表
3. ip地址，端口，路由表
4. iptables等网络资源

## 基本操作
linux 提供ip命令（安装iproute2）来对网络进行操作，使用ip netns命令可以操作network namespace

```
root@raspberrypi:/home/pi# ip netns help
Usage: ip netns list
       ip netns add NAME
       ip netns set NAME NETNSID
       ip [-all] netns delete [NAME]
       ip netns identify [PID]
       ip netns pids NAME
       ip [-all] netns exec [NAME] cmd ...
       ip netns monitor
       ip netns list-id

```
相对ip ro ,iptables, tc等命令而言，ip netns算是比较简单了的，就是增删改查。

- 查看namespace

`ip netns list`
没有添加过任何namespace执行上述命令不会有任何输出，需要新增一个namespace

- 新增namespace

`ip netns add netns1`

```
root@raspberrypi:/home/pi# ip netns add netns1
root@raspberrypi:/home/pi# ip netns list
netns1

```
新增完namespace之后，就可以对namespace进行操作了，可以在指定namespace执行命令，对网卡，路由等进行操作（不仅可以执行任何与网络的命令，还可以执行其他命令，比如bash）

- 对namespace进行操作
首先查看网卡信息，

```
root@raspberrypi:/home/pi# ip netns exec netns1 ip addr
1: lo: <LOOPBACK> mtu 65536 qdisc noop state DOWN group default qlen 1000
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00```

```
该namespace包含一个lo网卡，但是网卡处于DOWN状态。可以lo网卡UP起来

```
root@raspberrypi:/home/pi# ip netns exec netns1 ip link set lo up
root@raspberrypi:/home/pi# ip netns exec netns1 ip addr
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN group default qlen 1000
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
    inet 127.0.0.1/8 scope host lo
       valid_lft forever preferred_lft forever
    inet6 ::1/128 scope host
       valid_lft forever preferred_lft forever
```
up成功之后，就可以看到lo网卡的具体信息了。

为了方便，减少这么长的命令执行，使用ip netns exec命令执行可以执行一个bash程序，相当于在netns1内部执行bash，这样就不用每条命令头添加ip netns exec netns1了。

```
root@raspberrypi:/home/pi# ip netns exec netns1 bash --rcfile <(echo "PS1=\"ns1> \"")
ns1> route -n
Kernel IP routing table
Destination     Gateway         Genmask         Flags Metric Ref    Use Iface
ns1> ifconfig
lo: flags=73<UP,LOOPBACK,RUNNING>  mtu 65536
        inet 127.0.0.1  netmask 255.0.0.0
        inet6 ::1  prefixlen 128  scopeid 0x10<host>
        loop  txqueuelen 1000  (Local Loopback)
        RX packets 0  bytes 0 (0.0 B)
        RX errors 0  dropped 0  overruns 0  frame 0
        TX packets 0  bytes 0 (0.0 B)
        TX errors 0  dropped 0 overruns 0  carrier 0  collisions 0

ns1> iptables -t nat -nvL
Chain PREROUTING (policy ACCEPT 0 packets, 0 bytes)
 pkts bytes target     prot opt in     out     source               destination

Chain INPUT (policy ACCEPT 0 packets, 0 bytes)
 pkts bytes target     prot opt in     out     source               destination

Chain OUTPUT (policy ACCEPT 0 packets, 0 bytes)
 pkts bytes target     prot opt in     out     source               destination

Chain POSTROUTING (policy ACCEPT 0 packets, 0 bytes)
 pkts bytes target     prot opt in     out     source               destination
ns1>
```

## 网络连通性
有了上述的基本操作之后，namespace建好了，网卡也拉起来了，这时候可以针对namespace网络进行进一步思考，比如，是否可以和其他namespace进行通信。答案是不可以。

我们在讨论两个网络是否互通的时候，通常会说，如果在同一个网络上的，那么需要借助交换机，如果不在同一个主机上的，那么需要借助路由器。显然通过上述的操作，整个网络路由表为空，网卡也只有lo网卡，所以是无法与外界进行联系的。

linux network namespace与外界进行连通，还需要辅助veth设备，这部分将在[docker网络之veth设备部分](docker网络之veth设备.md)进行阐述。

