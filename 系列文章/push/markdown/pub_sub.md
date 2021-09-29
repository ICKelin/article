# 发布订阅模式
发布订阅，很多情况下直接称为pub/sub，在日常开发中用的比较多，无论是使用消息中间件还是做一些实时推送相关的服务，基本都用到pub/sub来解决问题。pub/sub模式包含三个角色:

- publisher: 发布者，用于发布消息
- subscriber: 订阅者，关注自己感兴趣的topic
- broker: 处于publisher和subscriber的消息代理人

整个模式核心的地方在于broker，broker连接publisher和subscriber，接收publisher发布的消息，并将消息发送给**指定**的订阅者，这点与生产-消费模式不一样，subscriber并不一定会得到全量的数据，broker只会给subscriber发送其关注的消息

## publisher实现
publisher实现可以多种多样，可以长连接，可以http，可以grpc，只要能把消息发送给broker即可，因为publisher负责发布，是主动的，跟broker不一定需要保持连接状态

## subscirber实现
subscriber实现比publisher要稍微困难一点，subscriber是消息被动接受者，也就意味着要跟broker保持连接状态，从而能够及时接收到消息，我个人认为从实时推送的角度而言，subscriber和broker之间的通信协议是一项关键的地方，会从以下几个方面进行考虑:

- 实现成本
- 流量消耗
- 电量消耗
- 多平台，比如web端

当然具体情况具体分析

## broker实现
broker实现可以很复杂，一方面要维护topic和subscriber的关系，另一方面，有些场景，特别是跟金钱打交道的场景，要考虑消息的不重不漏，消息重复或者消息丢失，可能都会造成资金，结算等问题，都是能让你丢饭碗的活。

broker实现也可以简化，只理会topic和subscriber的关系，只推实时数据，尽力而为的推送，这种场景一般对数据准确性要求不是非常严格，推送数据可以丢失，后续通过接口修正回来就行。

## 实现一个发布订阅的基础模块
在开始实时推送相关的技术之前，首先要解决实时推送用到的pub/sub相关的代码，这部分代码相对比较独立，写好了之后在后续开发当中集成进去就可以了。这部分代码已经上传到目录 `code/broker` 当中
