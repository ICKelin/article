## K8s service网络

### 起源

公司新开发的功能统一采用k8s部署，也因此了解了一些k8s的知识，由于自身对网络方向感兴趣，因此着重去k8s service网络相关的技术以及实现。

k8s主要通过service的方式对外提供服务，service屏蔽了pod网络信息，我个人理解k8s service就是个分布式的负载均衡器，在每个node上使用iptables在PREROUTING和OUTPUT上加入DNAT命令，或者使用ipvs等专门做负载均衡的软件。最终期望达到**在容器内部，通过service IP和端口，总能访问到一组pod**

### 为什么要设计service

首先来看看没用service或者k8s之前，我们是如何做的，不同公司做法可能不一样，以下只是我所在公司的做法。

首先公司内部采用微服务架构，服务发现使用consul，容器化部署，没有采用任何容器互联的方案，容器与容器之间通信使用的是宿主机的IP。那么需要解决以下问题:

- 服务注册的问题，如何将宿主机的IP注册到consul上
- 当依赖的服务出现重启或者重新发布时，自身如何感知到其变化

这两个问题都需要在程序内部去适配，也就是每个服务都需要处理这些与业务无关的内容，这也是sidecar之类的代理产生的一大原因。

针对第一个问题，我们额外使用[registrator](https://github.com/gliderlabs/registrator)组件注册到consul，针对第二个问题，由于我们使用的是consul，可以watch到key变化的情况，在服务ip发生改变时，内部再重新初始化一遍。

当然这两个都勉强能够解决，但是并不能很好的解决，第一个问题，需要在每个节点上事先部署registrator容器，随着项目演进，人员流动，后面进来的同事不一定了解这一细节。第二个问题，reload并不是那么简单，我们内部使用了grpc，grpc server地址变更处理不是很麻烦，但是其他组件，像kafka，监控等基础组件，reload可能就容易出错，而且每个同事都需要关注这点，我们现在大部分都是通过基础库做了一层封装。

k8s的service就很好的解决这个问题，软件开发里面有一句老话，大致意思是：没有什么是加一层中间件解决不了的。基础库其实也算一种中间层。k8s的service也是中间层，以不变应万变，以不变的service ip应付千变万化的pod ip，无论pod ip如何变，在集群内都可以通过service ip访问到变化后的pod ip，这样上面的感知容器IP变化的问题就可以在外部解决了，不用在每个服务内部编码处理，最终的目标是服务只处理业务，其他东西都期望放在外部处理。

那么问题就变成了下面两个问题：

- service ip如何映射pod ip
- service是如何感知到pod ip并更新上面问题当中的映射关系的

当然这两个问题都已经不需要我们解决了，k8s的组件kube-proxy已经解决了，我们需要研究的是他是如何解决的，k8s是很典型的sdn类的应用，包括控制平面和转发平面，控制平面提供上帝视角，管理整个集群，转发平面与控制平面对接，感知集群的变化，并应用到转发表当中。

k8s的service是建立在CNI的提供的功能基础之上的，没有CNI提供的POD与POD，POD与Node之间可以通过POD IP进行访问的功能，service的实现可能就完全不一样。

在CNI的基础之上，如果我们希望使用clusterIP的方式，那么会分配一个VIP（service ip）,在集群任何一个node上访问改VIP，最终都会被DNAT到POD IP。这样，无论pod ip如何改变，VIP是不变的，k8s通过kube-proxy和控制平面对接，感知pod ip变化，并重新DNAT到变化后的POD IP。

如果我们希望使用NodePort的方式，也即是通过集群当中任何一个Node IP和端口都能访问到，那么在集群的任何一个node上针对改端口的入口，出口流量，匹配上端口和协议之后，最终也是DNAT到POD IP。NodePort的目的是为了让不在集群当中的虚拟机也能够访问k8s内部网络。

>  k8s的service ip让我想起了两年前写的一个内网穿透软件，原理非常类似。
>
> 一开始使用的是tun/tap隧道，给每个客户端分配一个内网的ip地址，然后再利用nginx的七层代理功能，根据host代理到客户端的内网IP，从而实现了访问客户端，但是这仅仅只能解决七层代理，很多人希望使用tcp的，但是针对tcp我们很容易想到四层代理，也就是iptables，ipvs之类的，但是彼时最直接的做法是仿造Nginx，甚至想直接用nginx的tcp代理功能，但是最后没有，在本地监听一个端口，同样代理到客户端的内网IP。
>
> 接下来问题来了，用户反馈windows不好用，因为要额外安装tap驱动。而且要管理员权限，那就优化，怎么优化？跟k8s的service原理是一样的，我就不在使用隧道了，但是每个客户端还是分配了个内网IP，这个内网IP等价于k8s的service IP，除了服务器之外，任何一个地方都无法访问，那么我上层依旧不变，nginx依旧使用nginx，tcp的代理端口依旧保留监听。我内部在iptables上的OUTPUT加一条DNAT规则，所有目的地址是该网段的都DNAT到本机监听的端口，实现流量拦截，在通过getorigindst功能，获取到要访问的客户端IP以及端口。一切就能解决。

### service类型

service目前接触的主要有两种类型：

- clusterIP
- nodeport

ClusterIP是k8s的主要使用方式，主要是集群内部服务间会经常采用的方式，会创建一个service并构建端口映射，在集群内部既可以通过service ip和端口访问，也可以通过pod ip和端口访问。

clusterIP有个缺点，那就是不在集群内部是访问不了的，比如我司测试环境k8s集群在一个环境，但是部分老服务，比如接入层，是不部署在k8s当中的，而是在另外的VM上，这些VM可以和k8s的node通信，因此就需要能够通过node ip和端口访问到pod，于是就有了nodeport类型。

nodeport类型会使用node节点以及端口，并映射到一组pod ip，有了前面的知识我想应该可以知道一种实现的方式了——只需要在iptables的PREROUTING hook点当中加入DNAT规则，匹配到目的端口为xxxx的，DNAT到pod ip。这样就完成了nodeport。

当然实际使用当中，我们很少用nodeport，一来是历史原因，我们老的服务发现用的consul，没有全部迁移到k8s上，于是需要将k8s的service信息同步更新到consul上，但是运维在实现的过程中，只同步了clusterip类型的service，因此我们找运维帮忙打通虚拟机的pod ip地址段的路由，所以在任一环境都可以访问pod，自然就不需要nodeport了。

### ClusterIP的原理

接下来看clusterIP类型的service是如何实现的。

```
root@iZj6cce64o4g9pho18oib7Z:~# kubectl get svc -o wide
NAME            TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)        AGE   SELECTOR
kubernetes      ClusterIP   10.96.0.1        <none>        443/TCP        28d   <none>
nginx-service   NodePort    10.104.141.164   <none>        80:32311/TCP   28d   app=nginx
```

当前有两个service，第一个kubernetes这个是k8s创建的，nginx-service是我创建的，CNI使用的是flannel，关于flannel可以查看我个人写的关于flannel原理的文章。

首先来看iptables规则，先看OUTPUT。

```
root@iZj6cce64o4g9pho18oib7Z:~# iptables -t nat -nvL OUTPUT
Chain OUTPUT (policy ACCEPT 591 packets, 37644 bytes)
 pkts bytes target     prot opt in     out     source               destination
 1345 87100 KUBE-SERVICES  all  --  *      *       0.0.0.0/0            0.0.0.0/0            /* kubernetes service portals */
  669 40140 DOCKER     all  --  *      *       0.0.0.0/0           !127.0.0.0/8          ADDRTYPE match dst-type LOCAL
```

会看到有一条target为KUBE-SERVICES的跳转

```
root@iZj6cce64o4g9pho18oib7Z:~# iptables -t nat -nvL KUBE-SERVICES
Chain KUBE-SERVICES (2 references)
 pkts bytes target     prot opt in     out     source               destination
    0     0 KUBE-MARK-MASQ  tcp  --  *      *      !10.244.0.0/16        10.96.0.1            /* default/kubernetes:https cluster IP */ tcp dpt:443
    6   360 KUBE-SVC-NPX46M4PTMTKRN6Y  tcp  --  *      *       0.0.0.0/0            10.96.0.1            /* default/kubernetes:https cluster IP */ tcp dpt:443
```

其中KUBE-MARK-MASQ是一条打mark的chain，用来做SNAT的，此处可以暂时忽略。

```
root@iZj6cce64o4g9pho18oib7Z:~# iptables -t nat -nvL KUBE-MARK-MASQ
Chain KUBE-MARK-MASQ (7 references)
 pkts bytes target     prot opt in     out     source               destination
    0     0 MARK       all  --  *      *       0.0.0.0/0            0.0.0.0/0            MARK or 0x4000
root@iZj6cce64o4g9pho18oib7Z:~# iptables -t nat -nvL KUBE-POSTROUTING
Chain KUBE-POSTROUTING (1 references)
 pkts bytes target     prot opt in     out     source               destination
 1838  114K RETURN     all  --  *      *       0.0.0.0/0            0.0.0.0/0            mark match ! 0x4000/0x4000
    1    60 MARK       all  --  *      *       0.0.0.0/0            0.0.0.0/0            MARK xor 0x4000
    1    60 MASQUERADE  all  --  *      *       0.0.0.0/0            0.0.0.0/0            /* kubernetes service traffic requiring SNAT */
```

KUBE-SVC-NPX46M4PTMTKRN6Y 这个chain再往下则执行真正的DNAT

```
root@iZj6cce64o4g9pho18oib7Z:~# iptables -t nat -nvL KUBE-SVC-NPX46M4PTMTKRN6Y
Chain KUBE-SVC-NPX46M4PTMTKRN6Y (1 references)
 pkts bytes target     prot opt in     out     source               destination
   13   780 KUBE-SEP-IWN55A6BDZP3M4OC  all  --  *      *       0.0.0.0/0            0.0.0.0/0            /* default/kubernetes:https */
root@iZj6cce64o4g9pho18oib7Z:~# iptables -t nat -nvL KUBE-SEP-IWN55A6BDZP3M4OC
Chain KUBE-SEP-IWN55A6BDZP3M4OC (1 references)
 pkts bytes target     prot opt in     out     source               destination
    1    60 KUBE-MARK-MASQ  all  --  *      *       172.31.185.160       0.0.0.0/0            /* default/kubernetes:https */
   13   780 DNAT       tcp  --  *      *       0.0.0.0/0            0.0.0.0/0            /* default/kubernetes:https */ tcp to:172.31.185.160:6443
```

最后整理了下，经过

1. OUTPUT
2. KUBE-SERVICES
3. KUBE-SVC-NPX46M4PTMTKRN6Y（匹配目的地址和端口）
4. KUBE-SEP-IWN55A6BDZP3M4OC（DNAT）

k8s可能出于规则的扩展性以及复用考虑，创建了非常多的chain，iptables规则本身不是做网络相关开发的接触得就比较少，规则还写得这么绕，从维护的角度而言是非常不容易的。但是无论如何，最终的目的只有一个，匹配，然后DNAT。这是使用iptables实现的service的根本出发点。

### NodePort的原理

```shell
root@iZj6cce64o4g9pho18oib7Z:~# kubectl get svc -o wide
NAME            TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)        AGE   SELECTOR
kubernetes      ClusterIP   10.96.0.1        <none>        443/TCP        28d   <none>
nginx-service   NodePort    10.104.141.164   <none>        80:32311/TCP   28d   app=nginx
```

除了clusterIP之外，还有个nginx-service使用的是nodeport类型的service，依旧会分配一个cluster-ip: 10.104.141.164 nginx-service 的pod为带app=nginx标签的pod，包含了三个pod。

```shell
root@iZj6cce64o4g9pho18oib7Z:~# kubectl get pods  -o wide
NAME                                READY   STATUS    RESTARTS   AGE   IP            NODE                      NOMINATED NODE   READINESS GATES
nginx-deployment-66b6c48dd5-49x9s   1/1     Running   2          28d   10.244.1.10   izj6ccaz331dob0wr9fqsxz   <none>           <none>
nginx-deployment-66b6c48dd5-gffcq   1/1     Running   2          28d   10.244.1.12   izj6ccaz331dob0wr9fqsxz   <none>           <none>
nginx-deployment-66b6c48dd5-x27l5   1/1     Running   2          28d   10.244.1.11   izj6ccaz331dob0wr9fqsxz   <none>           <none>
```

NodePort的目的，是可以通过Node的IP和端口，访问到nginx。

```shell
root@iZj6cce64o4g9pho18oib7Z:~# iptables -t nat -nvL OUTPUT
Chain OUTPUT (policy ACCEPT 378 packets, 23146 bytes)
 pkts bytes target     prot opt in     out     source               destination
 1559 98568 KUBE-SERVICES  all  --  *      *       0.0.0.0/0            0.0.0.0/0            /* kubernetes service portals */
  943 56580 DOCKER     all  --  *      *       0.0.0.0/0           !127.0.0.0/8          ADDRTYPE match dst-type LOCAL
root@iZj6cce64o4g9pho18oib7Z:~# iptables -t nat -nvL KUBE-SERVICES|grep NODEPORTS
  358 20814 KUBE-NODEPORTS  all  --  *      *       0.0.0.0/0            0.0.0.0/0            /* kubernetes service nodeports; NOTE: this must be the last rule in this chain */ ADDRTYPE match dst-type LOCAL
root@iZj6cce64o4g9pho18oib7Z:~# iptables -t nat -nvL KUBE-NODEPORTS
Chain KUBE-NODEPORTS (1 references)
 pkts bytes target     prot opt in     out     source               destination
    1    60 KUBE-MARK-MASQ  tcp  --  *      *       0.0.0.0/0            0.0.0.0/0            /* default/nginx-service */ tcp dpt:32311
    1    60 KUBE-SVC-V2OKYYMBY3REGZOG  tcp  --  *      *       0.0.0.0/0            0.0.0.0/0            /* default/nginx-service */ tcp dpt:32311
root@iZj6cce64o4g9pho18oib7Z:~# iptables -t nat -nvL KUBE-SVC-V2OKYYMBY3REGZOG
Chain KUBE-SVC-V2OKYYMBY3REGZOG (2 references)
 pkts bytes target     prot opt in     out     source               destination
    1    60 KUBE-SEP-IL2K3FQ4GTSVJENH  all  --  *      *       0.0.0.0/0            0.0.0.0/0            /* default/nginx-service */ statistic mode random probability 0.33333333349
    0     0 KUBE-SEP-DFQCOZ4LW3OOKIRQ  all  --  *      *       0.0.0.0/0            0.0.0.0/0            /* default/nginx-service */ statistic mode random probability 0.50000000000
    0     0 KUBE-SEP-ICPADH3OVWVSSLL6  all  --  *      *       0.0.0.0/0            0.0.0.0/0            /* default/nginx-service */
root@iZj6cce64o4g9pho18oib7Z:~# iptables -t nat -nvL KUBE-SEP-IL2K3FQ4GTSVJENH
Chain KUBE-SEP-IL2K3FQ4GTSVJENH (1 references)
 pkts bytes target     prot opt in     out     source               destination
    0     0 KUBE-MARK-MASQ  all  --  *      *       10.244.1.14          0.0.0.0/0            /* default/nginx-service */
    1    60 DNAT       tcp  --  *      *       0.0.0.0/0            0.0.0.0/0            /* default/nginx-service */ tcp to:10.244.1.14:80
root@iZj6cce64o4g9pho18oib7Z:~#

```



<img src="/Users/ickelin/ownCloud/系列文章/docker/images/k8s-nodeport.jpg" style="zoom:50%;" />

Node的iptables规则也经历了多次跳转。

1. OUTPUT
2. KUBE-SERVICES
3. KUBE-NODEPORTS
4. KUBE-SVC-V2OKYYMBY3REGZOG（匹配端口）
5. KUBE-SEP-IL2K3FQ4GTSVJENH、KUBE-SEP-DFQCOZ4LW3OOKIRQ、KUBE-SEP-ICPADH3OVWVSSLL6 这三条分别对应三个POD的DNAT规则。

### iptables之外

service除了基于iptables实现之外，还能基于ipvs实现，iptables管理起来比较复杂，维护起来更是需要对linux网络有一定的经验，另外一点是iptables的效率问题，iptables根据逐条匹配，如果规则多了之后可能会存在效率问题，不过我目前觉得大部分公司，应该都还没有遇到所谓的iptables的匹配效率问题，但是有更加优秀的解决方法用上了更好。

