# 保姆级教程——如何实现内网NAS访问
在讲解如何访问NAS之前，我先来说下为什么需要访问内网NAS。

首先处于成本考虑，很多NAS系统都是部署在企业内网的，离开公司就无法访问，现在是高效率办公的时代，可能随时随地都需要用到NAS内部的文件，资料等，离开公司就不能工作的年代已经不复存在。

其次出于数据安全考虑，企业通常不会随便将系统对外，那么像一些像找ISP开具公网IP，使用动态dns，甚至使用一些内网穿透工具等基本不被企业接受。

最终我们归结客户的痛点无非两个：

1. 把网络打通，能够访问到NAS
2. 把安全性做好，免得被攻击或者数据泄漏

我们有两款产品可以解决这个问题，一款是基于零信任理念的网关，一款是本文提到的SD-WAN组网。

本系列包括以下文章，感兴趣的朋友也可以多参考参考。

- [SD-WAN组网系列：产品介绍](https://www.beyondnetwork.net/2023/03/06/sdwan%e4%ba%a7%e5%93%81%e4%bb%8b%e7%bb%8d/)
- [SD-WAN组网系列：保姆级教程——如何快速配置组网](https://doc.beyondnetwork.net/#/sdwan/quickstart)
- [SD-WAN组网系列：保姆级教程——如何做跨云组网](https://www.beyondnetwork.net/2023/03/22/sd-wan%e8%b7%a8%e4%ba%91%e7%bb%84%e7%bd%91/)
- [SD-WAN组网系列：保姆级教程——如何访问内网NAS(本文)](https://www.beyondnetwork.net/2023/03/29/sd-wan%e5%ae%9e%e7%8e%b0%e5%86%85%e7%bd%91%e7%a9%bf%e9%80%8f%e6%8a%80%e6%9c%af%e5%8e%9f%e7%90%86/)
- [SD-WAN组网系列：保姆级教程——如何使用实现企业分支互联](https://www.beyondnetwork.net/2023/03/28/sd-wan%e5%a6%82%e4%bd%95%e5%ae%9e%e7%8e%b0%e4%bc%81%e4%b8%9a%e5%88%86%e6%94%af%e7%bb%84%e7%bd%91/)
- [SD-WAN组网系列：保姆级教程——如何实现企业网(员工，企业分支，公有云)]()
- [SD-WAN组网系列：保姆级教程——如何实现全球组网]()

## 方案制定
基于SD-WAN的产品解决方案思路为将企业内网和公有云网关组成一个虚拟局域网，基于此组网架构，能够完成从公有云访问内网，那么就能够在公有云上访问内网NAS。

有了这个基础，我们可以有两种方式向客户提供服务：

1. 在公有云网关上配置iptables转发，将公网IP和端口映射到NAS的IP和端口，操作简单，适用于个人或者小团队，安全得不到保证
2. 通过app把流量转发到公有云网关，再在公有云网关上把流量转发到内网，这种方式由于采用了app转发流量，而不是直接使用公网，相对会安全很多。

在本文中，出于简单考虑，我们使用第一种方式进行讲解，如果您希望采用方式2，可以与我们[取得联系](https://www.beyondnetwork.net/about-us)。

## 方案实施

**第一步：配置虚拟局域网和子网**

和同系列其他教程一样，操作，这里不再赘述了，感兴趣可以查看[快速入门文档](https://doc.beyondnetwork.net/#/sdwan/quickstart) 里面有非常详细的教程


第二步：配置iptables转发

假设我们的内网NAS的IP是192.168.1.10，端口是3000，那么我们需要配置如下iptables规则进行转发。

```
iptables -t nat -I PREROUTING -p tcp --dport 30000 -j DNAT --to 192.168.1.10:3000
iptables -t nat -I POSTROUTING -p tcp --dport 3000 --dst 192.168.1.10 -j MASQUERADE
```

如果有需要的话需要开启ip_forward以及iptables forward链的ACCEPT功能

开启ip_forward
```
vim /etc/sysctl.conf
net.ipv4.ip_forward = 1
```

开启iptables forward默认侧露为ACCEPT
```
 iptables -t filter -P FORWARD ACCEPT
```

完成如上配置之后，我们就可以通过浏览器打开 `http://你的公网IP:30000` 访问到内网NAS了。

## 方案总结
SD-WAN能够解决类似内网穿透的场景，也能实现安全的访问内网，但是相比零信任产品而言，我目前认为SD-WAN在该场景下属于能用，但是不是最优的方案，如果您有类似场景，可以跟我们取得联系，我们根据您的实际情况提出一个符合实际的解决方案。


