## flannel简介
之前文章有提到容器之间通信的方式主要有三种：

1. 通过宿主机端口映射，也就是NAT
2. 通过路由的方式，将其他容器地址加入到host机
3. 通过隧道封装，将ip包封装然后进行转发

第一种方式不算容器通信，容器之间通信通常的说法是容器之间可以直接使用容器IP进行通信，第二种方式比较典型的代表是calico，效率较高，隧道方式主要代表是flannel，flannel是老牌容器通信解决方案。

flannel除了隧道方式之外，本身也支持路由方式，也就是其host-gw模式。

### flannel的基本玩法
flannel本身玩法很简单，初始化在etcd里面存了个大的子网，在每个host机上运行一个flanneld的进程，flanneld负责两个方面的工作：

- 从大的子网当中申请一个未被使用的子网，并将这个子网应用在docker当中，每个容器又会在这个子网当中申请一个地址。
- 将子网当中的数据包进行转发，采用udp模式的会被路由到tun网卡，flanneld读取tun网卡数据并发到对端，采用host-gw模式的会被直接路由到对端，不经过overlay封装

flannel源码当中，以上两个功能在分别为subnet和backend，如果可以，你也可以开发自己的backend和subnet模块，只要实现对应的接口即可。

![flannel](images/packet-01.png)

### flannel的几种backend
flannel的backend主要包括三种，分别是udp，vxlan，host-gw，除了这三种模式之外，flannel还针对云服务厂商提供了对应的backend，比如aws和阿里云。

udp和vxlan backend都是隧道封装，区别在于进入协议栈多少次，vxlan是在内核当中实现的，udp是基于tun网卡，vxlan在转发速度上取胜。

host-gw是一种类似calico的纯路由技术，我们要两台机器能够互通，从网络层面来说是A机器有到B机器的一条路由。host-gw类似这种思路，A机器和B机器网络出于一个局域网，A机器的容器和B机器的容器本身是没有可达的路由的，但是A机器的容器数据包出去时会到达A，那么只需要在A当中添加B的容器的ip地址，那么A机器下面的容器数据包就能到达B，然后进入B的协议栈再通过veth进入到B的容器。同理，B机器下面的容器回包给A机器下面的容器，也需要具备相应的路由。

host-gw的优势是快，相比较udp和vxlan而言，没有任何的封装，缺陷是机器之间必须要在二层网络当中，不然连路由都加不通，另外一个是路由条目可能会比较多，但是flannel这种成熟方案在路由管理方面的bug应该被修复得差不多了。

接下来几篇文章会从协议以及源码层面介绍flannel以及flannel的的host-gw，udp，vxlan 三种backend。