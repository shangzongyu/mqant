# mqant

mqant 是一款基于 Golang 语言的简洁，高效，高性能的分布式微服务游戏服务器框架，研发的初衷是要实现一款能支持高并发，高性能，高实时性，的游戏服务器框架，也希望 mqant 未来能够做即时通讯和物联网方面的应用

##	特性

1. 高性能分布式
2. 支持分布式服务注册发现，是一款功能完整的微服务框架
3. 基于 golang 协程，开发过程全程做到无 callback 回调，代码可读性更高
4. 远程 RPC 使用 nats 作为通道
5. 网关采用 MQTT 协议，无需再开发客户端底层库，直接套用已有的 MQTT 客户端代码库，可以支持 IOS,Android,websocket,PC 等多平台通信
6. 默认支持 mqtt 协议，同时网关也支持开发者自定义的粘包协议

## 文档

- [在线文档](https://liangdas.github.io/mqant/)
- [在线文档-访问不了用这个](http://docs.mqant.com/)

## 模块

> 将不断加入更多的模块

- [mqant 组件库](https://github.com/shangzongyu/mqant-modules)
  - 短信验证码
  - 房间模块
- [压力测试工具：armyant](https://github.com/shangzongyu/armyant)

## 社区贡献的库

- [mqant-docker](https://github.com/bjfumac/mqant-docker)
- [MQTT-Laya](https://github.com/bjfumac/MQTT-Laya)

## 应用案例

[恰玩-实时互动游戏社交 app](https://tiyfr.com/)

## 演示示例

- mqant 项目只包含 mqant 的代码文件
- mqantserver 项目包括了完整的测试demo代码和 mqant 所依赖的库
- 如果你是新手可以优先下载 mqantserver 项目进行试验


[在线 Demo 演示](http://www.mqant.com/mqant/chat/)【[源码下载](https://github.com/shangzongyu/mqantserver)】

[多人对战吃小球游戏 (绿色小球是在线玩家，点击屏幕任意位置移动小球，可以同时开两个浏览器测试，支持移动端)](http://www.mqant.com/mqant/hitball/)【[源码下载](https://github.com/shangzongyu/mqantserver)】


## 贡献者

欢迎提供 dev 分支的 pull request

bug 请直接通过 issue 提交

凡提交代码和建议，bug 的童鞋，均会在下列贡献者名单者出现

1. [xlionet](https://github.com/xlionet)
2. [lulucas](https://github.com/lulucas/mqant-UnityExample)
3. [c2matrix](https://github.com/c2matrix)
4. [bjfumac【mqant-docker】[MQTT-Laya]](https://github.com/bjfumac)
5. [jarekzha【jarekzha-master】](https://github.com/jarekzha)


## 打赏作者

![](http://docs.mqant.com/images/donation.png)

