# docker网络之容器模式

之前文章中提到docker网络的三大基础，namespace，veth设备，网桥。

使用namespace技术来实现网络隔离，但是如果单纯的封闭在自己的小岛上，容器技术就不会这么流行，为了打开容器的大门，docker使用了veth设备对，veth将容器namespace与外界打通，在网络打通之后，为了更加灵活，于是用了linux网桥技术。

有了这三项技术基础之后，开始提到docker网络模式中none模式，然后基于none模式之上，利用namespace，veth，在none模式下建立一个bridge模式的雏形。本来想将bridge，container，host三种模式分开写，仔细考虑之后觉得没有太大必要，于是在此文中将这三种模式一并完成。

# host模式
host模式没有创建namespace，也没有创建veth网卡，更谈不上桥接到docker0网卡。host模式和host机公用同一个namespace，但是进程，文件系统还是相互隔离的。

- 创建一个host模式的容器
```
root@raspberrypi:/home/pi# docker run -d --network=host --restart=always nginx
9d812d6519e1a49606bf4ceb7a3cc265347ba3be686a65edea7c1401604489f6
root@raspberrypi:/home/pi# docker ps
CONTAINER ID        IMAGE                      COMMAND                  CREATED             STATUS                PORTS                    NAMES
9d812d6519e1        nginx                      "/docker-entrypoint.…"   7 seconds ago       Up 3 seconds                                   dazzling_meninsky
```

查看namespace信息
```
root@raspberrypi:/home/pi# ls -l /proc/3215/ns/net
lrwxrwxrwx 1 root root 0 8月  17 22:09 /proc/3215/ns/net -> net:[4026532766]
root@raspberrypi:/home/pi# docker inspect 9d81|grep Pid
            "Pid": 25582,
            "PidMode": "",
            "PidsLimit": null,
root@raspberrypi:/home/pi# ls -l /proc/25582/ns/net
lrwxrwxrwx 1 root root 0 8月  17 22:06 /proc/25582/ns/net -> net:[4026532766]
```

查看主机上某个进程的namespace
```
root@raspberrypi:/home/pi# ls -l /proc/3215/ns/net
lrwxrwxrwx 1 root root 0 8月  17 22:09 /proc/3215/ns/net -> net:[4026532766]
```

可以容器的namespace和host机的是一样的。

host模式最大的好处是没有性能损耗，因为不用走docker0再过一遍协议栈。缺点有:
- 由于没有创建namespace，所以host机的网络会影响到容器的网络
- 端口，ip地址共用，存在端口冲突

## container模式
container模式类似host模式，host模式使用了host机的namespace，container模式使用了其他container的namespace。

## bridge模式
bridge被广泛使用的一种容器网络模式，在[docker网络之none模式](docker网络之none模式.md)当中手动创建了bridge模式的雏形。bridge模式依赖docker0网桥和veth设备。

bridge模式启动的容器，在容器启动时，会创建namespace，创建veth设备对，并将veth设备对的一端加入容器的namespace，另外一端桥接到docker0当中，docker daemon会容器设置ip，并设置下一跳是docker0。

在与外网通信时，docker0网桥作为下一跳，当与同一host机下的容器通信时，docker0为交换机，做拷贝mac地址学习，拷贝数据帧，数据帧泛洪等功能。

bridge模式的优势在于网络隔离，容器具备自己的ip地址和所有端口信息，不存在容器间端口冲突的问题

bridge模式的问题主要也是在于网络隔离，由于引入docker0，数据包往外发送时，先经过docker0，进入host机的协议栈，经过netfilter，路由子系统，再出去，可能会造成一定的性能损耗。
除了性能损耗而言之外，还有一个最大的问题是访问容器内部网络，在外部是无法直接通过容器的ip路由的，所以容器间通信就会变复杂。我目前了解到的主要有三种方式：

- 通过host机做端口映射，原理类似家庭路由器配置的端口映射，host机是可以访问容器的，那么通过host机的一个端口映射到容器的一个端口，从而实现访问容器
- 通过ip隧道的方式，类似vpn
- 通过路由的方式，先路由到host机，由host机帮你发给容器

这三种也是一些容器间通信项目，比如flannel，weave，calico等的解决方法，在后续将会对三种技术进行阐述。