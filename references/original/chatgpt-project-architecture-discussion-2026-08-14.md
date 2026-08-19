可以，而且这个项目如果目标真的是最多 100 万设备接入，我建议一开始就不要按“普通视频管理平台”的思路设计，而要按“海量设备接入 + 分布式流媒体基础设施”来设计。

我的核心结论是：

控制面用 Go，媒体面用 C++，前端用 TypeScript；GB28181/SIP 接入层可以采用 Kamailio/OpenSIPS + 自研 Go 业务层；媒体引擎优先采用 ZLMediaKit 作为基础能力进行二次开发，而不是从 RTP/RTSP/WebRTC 底层全部自研。

GB/T 28181-2022 相比 2016 版已经增加/强化了 H.265、AAC、RTP 时间戳、设备控制等内容，所以设计时建议直接以 28181-2022 为基线，而不是只兼容老版本。

⸻

一、我建议的总体架构

不要做成：

前端
  ↓
Java/Go后端
  ↓
流媒体服务器
  ↓
摄像头

而应该做成：

                         ┌─────────────────────────┐
                         │      Web / APP / 第三方 │
                         └────────────┬────────────┘
                                      │
                         HTTPS / WebSocket / WebRTC
                                      │
                    ┌─────────────────▼─────────────────┐
                    │            API Gateway             │
                    │       鉴权 / 限流 / 路由 / 租户     │
                    └─────────────────┬─────────────────┘
                                      │
        ┌─────────────────────────────┼────────────────────────────┐
        │                             │                            │
        ▼                             ▼                            ▼
┌──────────────┐              ┌──────────────┐             ┌──────────────┐
│ 设备管理服务 │              │ 流管理服务   │             │ 运维管理服务 │
│ Device       │              │ Stream       │             │ OPS          │
└──────┬───────┘              └──────┬───────┘             └──────┬───────┘
       │                             │                            │
       └────────────────┬────────────┴────────────────────────────┘
                        │
                 Kafka / Event Bus
                        │
        ┌───────────────┼──────────────────┐
        │               │                  │
        ▼               ▼                  ▼
   Redis Cluster    PostgreSQL        ClickHouse
   在线状态/路由     设备/组织/配置      运行指标/日志/历史
        │
        │
══════════════════════════════════════════════════════════════════
                         接 入 层
══════════════════════════════════════════════════════════════════
        GB28181             RTSP             ONVIF        其他协议
           │                  │                 │              │
           ▼                  ▼                 ▼              ▼
   ┌─────────────────────────────────────────────────────────────┐
   │                  Protocol Gateway Cluster                  │
   │ SIP / RTP / RTSP / ONVIF / 私有协议 Adapter                │
   └───────────────────────────┬─────────────────────────────────┘
                               │
                               ▼
                    ┌───────────────────────┐
                    │   Media Worker Cluster │
                    │   C++ / ZLMediaKit     │
                    │                       │
                    │ RTP/PS解析             │
                    │ RTSP                   │
                    │ WebRTC                 │
                    │ RTMP                   │
                    │ HLS                    │
                    │ 录像                   │
                    │ 转协议                 │
                    │ 转码                   │
                    └───────────┬───────────┘
                                │
                ┌───────────────┼───────────────┐
                ▼               ▼               ▼
            实时播放          录像              级联
          WebRTC/RTSP       Object Storage     GB28181

这里最重要的一点是：

控制面和媒体面必须彻底分开。

这是 100 万规模和 1 万规模系统最重要的架构区别之一。

⸻

二、100 万到底应该怎么理解

“最大支持 100 万”这个指标一定要拆成至少 5 个指标。

例如：

指标    建议明确
注册设备数    1,000,000
在线设备数    1,000,000
在线视频通道    例如 500,000
同时拉流数    例如 100,000
单设备同时观看数    1～N
同时录像数    例如 100,000
PTZ 控制并发    例如 10,000/s
SIP 信令峰值    例如 10,000～50,000/s
RTP 带宽    根据码率计算

这个区别非常关键。

例如：

情况 A

100 万摄像头在线，但是只有 1 万路正在观看。

这个系统其实是：

海量连接 + 中等媒体并发

难度可控。

情况 B

100 万摄像头同时 1080P 发送视频。

假设平均：

2 Mbps/路

那么：

1,000,000 × 2 Mbps
= 2,000,000 Mbps
= 2 Tbps

这已经不是普通服务器集群问题，而是数据中心级网络、存储、交换机、带宽和流媒体基础设施问题。

如果平均 4 Mbps：

4 Tbps

所以：

100 万设备在线可以作为平台能力目标；
100 万路同时视频传输必须单独定义为另一档容量目标。

⸻

三、为什么我建议 Go + C++，而不是单一语言

1. Go：做控制面

我会优先选择：

Go 1.24/1.25 系列

用 Go 开发：

设备管理
组织管理
用户/权限
租户
设备注册管理
设备目录
国标目录
SIP业务逻辑
设备控制
PTZ
云台
录像查询
流生命周期管理
任务调度
集群调度
设备健康检测
API
WebSocket
事件系统
运维
监控

Go 的优点非常适合这个场景：

goroutine
channel
网络IO
大量长连接
高并发
微服务
交叉编译
容器化

尤其是 100 万设备的：

REGISTER
KeepAlive
MESSAGE
Catalog
DeviceInfo
Alarm
Notify

这些本质上都是大量网络 IO，非常适合 Go。

⸻

四、但是媒体流千万别用 Go 全部自己写

这是我非常明确的建议。

不要尝试自己用 Go 写：

RTP
PS
RTSP
WebRTC
H264
H265
编码器
转码器
Jitter Buffer
NACK
重传
SRTP

然后自己构建一个“超级流媒体服务器”。

因为这是非常大的坑。

⸻

五、媒体层：C++ + ZLMediaKit

我会优先考虑：

ZLMediaKit + 自研扩展

ZLMediaKit 本身已经支持：

GB28181
RTP
RTSP
RTMP
WebRTC
HLS
HTTP-FLV
WebSocket-FLV
SRT
STUN/TURN
MP4
录制
协议转换
集群

而且它是 C++11，高性能异步网络模型，同时支持 x86、ARM、龙芯、申威、RISC-V 等架构，这一点对于你前面提到的国产化服务器迁移/国产 CPU 环境尤其有价值。

它还提供 C API、REST API 和 WebHook，可以作为独立 MediaServer 使用，也可以作为 SDK 嵌入自己的平台。

因此我不会让你的 Go 团队重新造一个：

“国标流媒体引擎”

而是：

                   Go
             控制 / 业务 / 调度
                    │
               REST / API
                    │
                    ▼
         ┌──────────────────┐
         │   Media Worker   │
         │  ZLMediaKit/C++  │
         └──────────────────┘
                    │
           RTP / RTSP / WebRTC

⸻

六、GB28181 接入层，我建议单独设计

这里是整个系统最核心的部分。

不要把：

SIP
RTP
设备管理
流媒体
业务

全揉进一个程序。

应该拆：

                Internet / 专网
                       │
                       ▼
             ┌─────────────────┐
             │ SIP Edge Cluster│
             │ Kamailio/OpenSIPS│
             └────────┬────────┘
                      │
              SIP分发 / 路由
                      │
        ┌─────────────┼─────────────┐
        ▼             ▼             ▼
    GB Gateway 1  GB Gateway 2  GB Gateway N
        │             │             │
        └─────────────┼─────────────┘
                      │
                      ▼
                 Stream Manager
                      │
                      ▼
                Media Worker

Kamailio 的 Dispatcher 模块本身就是面向 SIP 流量做负载均衡、Hash 分发和高负载 SIP 流量调度的。

OpenSIPS 同样具备 SIP Dispatcher、Dialog Cluster 等能力，也可以作为 SIP 接入层。当前 OpenSIPS 4.0 已发布。

所以我更倾向：

Kamailio/OpenSIPS = SIP 基础设施

Go = 28181 业务控制

ZLMediaKit = RTP/媒体基础设施

这样职责非常清晰。

⸻

七、你真正需要自己开发的是“28181 平台层”

不要把大量精力花在 RTP 底层。

你真正应该拥有自己的：

Device Model

例如：

Device
├── deviceId
├── name
├── manufacturer
├── model
├── firmware
├── ip
├── port
├── transport
├── domain
├── status
├── registerTime
├── lastKeepalive
├── expires
├── protocol
└── capabilities

⸻

Channel Model

Device
   │
   ├── Channel
   │       ├── Main Stream
   │       ├── Sub Stream
   │       ├── Mobile Position
   │       └── PTZ
   │
   ├── Channel
   └── Channel

并且一定要把：

设备 ≠ 通道 ≠ 流 ≠ 会话

这四个概念分开。

这是很多视频平台后面做大之后非常容易踩的坑。

⸻

八、建立统一的“媒体抽象层”

以后你说：

播放摄像头 34020000001320000001

业务层不应该知道它到底是：

GB28181
RTSP
ONVIF
Hikvision
Dahua
GB35114
私有SDK

而应该统一：

Stream
{
    stream_id
    device_id
    channel_id
    protocol
    codec
    source
    media_server
    session
}

例如：

GB28181
       ↓
GB Adapter
       ↓
       Stream
       ↑
RTSP Adapter
       ↑
ONVIF Adapter
       ↑
Other Adapter

这一步对你未来扩展协议非常重要。

⸻

九、未来支持其他摄像头协议，要采用 Adapter 架构

我建议定义：

ProtocolAdapter

比如：

interface ProtocolAdapter {
    Register()
    Unregister()
    GetDevice()
    GetChannels()
    StartStream()
    StopStream()
    PTZControl()
    DeviceControl()
    QueryRecord()
    SubscribeEvent()
}

然后：

GB28181 Adapter
RTSP Adapter
ONVIF Adapter
Hikvision Adapter
Dahua Adapter
Other Adapter

这样以后增加一种协议，不需要修改整个系统。

⸻

十、媒体层最重要的设计：尽量“不转码”

千万不要默认：

摄像头
 ↓
解码
 ↓
编码
 ↓
播放

而要默认：

摄像头
 ↓
RTP
 ↓
PS Demux
 ↓
H264/H265
 ↓
封装转换
 ↓
WebRTC/RTSP

也就是：

能 Remux 就 Remux，能直通就直通。

因为：

转码 = CPU/GPU 大量消耗

而：

转协议 = 主要是网络 I/O + 封装处理

这是视频平台成本差异非常大的地方。

⸻

十一、Web 前端推荐 WebRTC

你这个平台如果主要面向现代 Web：

浏览器
   ↓
WebRTC
   ↓
Media Server
   ↓
GB28181
   ↓
Camera

而不是：

浏览器
 ↓
HLS

因为监控平台通常对实时性比较敏感。

可以设计成：

GB28181 H264
     ↓
    RTP
     ↓
Media Server
     ↓
 WebRTC
     ↓
 Browser

WebRTC 这一层也可以采用 Pion 作为 Go 侧能力，它是纯 Go 实现的 WebRTC 栈，支持 RTP/RTCP、H.264、VP8/VP9、NACK、带宽估计等能力。

不过如果已经大量使用 ZLMediaKit，我不会再同时堆一套复杂 WebRTC Media Server，而是优先让 ZLMediaKit 承担媒体转换。

⸻

十二、H.264 / H.265 一定要重点设计

28181-2022 已经正式增加了 H.265、AAC 等支持，所以架构不能只考虑 H.264。

尤其要考虑：

Camera
  │
  ├── H264
  └── H265

到：

WebRTC

这里会出现浏览器兼容和硬件能力差异。

所以建议：

H264
 ↓
尽量直接 WebRTC

而：

H265
 ↓
根据终端能力
 ├── 原码流
 ├── H265 WebRTC
 └── H265 → H264

转码应该是可插拔能力，而不是主链路默认能力。

⸻

十三、100 万设备的核心：Cell/Shard 架构

这是我非常推荐你采用的一种架构。

不要：

1个超级集群

而采用：

Cell Architecture / 分区单元架构

例如：

                 Global Control Plane
                         │
       ┌─────────────────┼─────────────────┐
       ▼                 ▼                 ▼
    Cell-01           Cell-02           Cell-03
    30K设备            30K设备            30K设备
       │                 │                 │
    Media               Media             Media
    SIP                 SIP               SIP
    Redis               Redis             Redis

比如：

1 Cell = 20,000 ~ 50,000 devices

100 万设备：

20K / Cell
→ 50 Cells
50K / Cell
→ 20 Cells

具体值不要一开始拍脑袋定，要通过你的设备厂商、KeepAlive 周期、SIP 行为、媒体并发做压测确定。

⸻

十四、设备应该通过 Hash 固定到 Cell

比如：

hash(deviceId) → Cell

得到：

34020000001320000001
        ↓
hash
        ↓
Cell-17

以后：

REGISTER
MESSAGE
KEEPALIVE
INVITE
BYE
PTZ
Catalog
Alarm

尽量都进入同一个 Cell。

这样可以极大降低分布式状态同步压力。

⸻

十五、Redis 应该干什么

Redis 不要成为“所有业务数据数据库”。

它主要负责：

设备在线状态
设备 → Cell
设备 → SIP Gateway
设备 → MediaServer
Channel → Stream
Stream → MediaServer
Session
分布式锁
短期缓存
限流

例如：

device:34020000001320000001
    ↓
cell-17
gateway-17-03
media-17-08

⸻

十六、数据库推荐 PostgreSQL

核心业务数据：

设备
组织
通道
权限
用户
租户
配置
录像索引
设备关系
国标目录
报警
任务

我会优先考虑：

PostgreSQL

因为它的分区、索引、事务能力比较完整，而且后续做国产数据库适配也比较容易。

PostgreSQL 当前版本支持声明式 partitioning，包括 range/list/hash 分区，可以用来组织大型设备和历史数据。

但是：

不要把实时设备状态全部放 PostgreSQL。

⸻

十七、运维数据不要全塞 PostgreSQL

例如：

设备在线率
CPU
Memory
网络流量
SIP请求次数
RTP丢包
Jitter
RTT
码率
播放次数
异常次数
设备上下线

这类时序/分析数据建议单独：

ClickHouse

或者：

Prometheus + VictoriaMetrics

架构：

业务数据
   ↓
PostgreSQL
实时状态
   ↓
Redis
指标
   ↓
Prometheus/VictoriaMetrics
历史分析
   ↓
ClickHouse

⸻

十八、消息系统建议 Kafka

比如：

DeviceRegistered
DeviceOffline
DeviceOnline
StreamStarted
StreamStopped
DeviceAlarm
DeviceCatalogChanged
RecordCreated
PTZCommand
MediaServerChanged

都不要靠 RPC 层层同步。

可以：

                Kafka
                  │
       ┌──────────┼─────────┐
       ▼          ▼         ▼
  Device Service  Alarm   OPS

Kafka 本身就是为分布式、可扩展、容错的事件流处理而设计的，适合这种设备状态和平台事件总线场景。

⸻

十九、录像系统要单独设计

录像不要：

MediaServer → PostgreSQL

而是：

Camera
  ↓
Media Server
  ↓
Recorder Worker
  ↓
MP4 / fMP4 / TS
  ↓
Object Storage

存储可以：

Ceph
MinIO
国产对象存储
NAS
SAN

数据库只保存：

record_id
device_id
channel_id
start_time
end_time
storage_node
object_key
file_size

⸻

二十、运维系统其实是这个项目的大头之一

你前面提到“运维”，这里我建议从一开始就作为一级模块设计。

至少需要：

设备层

在线
离线
注册失败
KeepAlive超时
SIP异常
RTP无流
码率异常
丢包
音视频异常

媒体层

媒体服务器CPU
GPU
内存
连接数
RTP包
带宽
输入码率
输出码率
WebRTC连接
RTSP连接
转码任务
录像任务

SIP层

REGISTER TPS
MESSAGE TPS
INVITE TPS
BYE TPS
404
408
486
500
503

设备质量

甚至建议做：

设备健康评分

例如：

在线率       30%
视频稳定性   30%
网络质量     20%
信令质量     10%
码率稳定性   10%

最终：

设备健康度 = 87

这对百万设备平台非常有价值。

⸻

二十一、最值得设计的是“设备生命周期”

不要只设计“添加设备”。

完整生命周期应该是：

未注册
   ↓
注册中
   ↓
在线
   ↓
目录同步
   ↓
能力同步
   ↓
流检测
   ↓
正常运行
   ↓
异常
   ↓
恢复
   ↓
离线

所有过程产生事件：

DeviceEvent

这样后面的运维、告警、审计都会非常容易。

⸻

二十二、控制接口也应该统一

例如 PTZ：

POST /api/v1/devices/{id}/channels/{id}/ptz

业务层：

PTZService
      ↓
ProtocolAdapter
      ↓
GB28181
      ↓
SIP MESSAGE

以后：

ONVIF PTZ

只需要：

ProtocolAdapter

发生变化。

⸻

二十三、推荐技术栈

我会给你定成下面这样：

层    技术
前端    Vue 3 + TypeScript
Web 播放    WebRTC
API    Go
控制面    Go
设备管理    Go
28181 业务    Go
SIP Edge    Kamailio / OpenSIPS
RTP/媒体    C++
Media Server    ZLMediaKit
转码    FFmpeg
WebRTC    ZLMediaKit / Pion
缓存    Redis Cluster
MQ    Kafka
业务数据库    PostgreSQL
大数据    ClickHouse
指标    Prometheus + VictoriaMetrics
日志    Loki / Elasticsearch
对象存储    MinIO / Ceph
容器    Kubernetes
API    REST + WebSocket
服务发现    Kubernetes / etcd
配置    Nacos / etcd
CI/CD    GitLab CI
容器镜像    Harbor

⸻

二十四、如果考虑国产化，我还会再做一个变化

鉴于你的场景本身就是国产化服务器迁移，我不建议：

“直接使用大量 x86 专属依赖。”

而是把整个系统设计成：

                    Application
                        │
               ┌────────┴────────┐
               │                 │
              Go                C++
               │                 │
          Linux ABI          Linux ABI
               │                 │
        ┌──────┴─────────┐ ┌─────┴──────┐
        │                │ │            │
       ARM64          x86_64         国产CPU
        │
   Media / ZLMediaKit

ZLMediaKit 官方项目本身明确支持 ARM、龙芯、申威、RISC-V 等多个体系结构，这对国产化部署比较友好。

这里尤其要控制：

CGO依赖
汇编优化
CPU SIMD
硬件编码器
FFmpeg编译参数
OpenSSL
网卡驱动
内核版本
NUMA

否则后面国产化迁移的时候，媒体服务很容易变成最大的障碍。

⸻

二十五、我不建议的几种架构

方案 1：全部 Java

Java
  ├── SIP
  ├── RTP
  ├── RTSP
  ├── WebRTC
  └── Media

我不建议。

业务层可以，但媒体层不是它最擅长的领域。

⸻

方案 2：全部 Go

比全部 Java 更好，但我仍然不建议把高性能媒体层全部自己写。

⸻

方案 3：全部 C++

也不建议。

业务开发速度会明显下降，而且设备业务、权限、运维、API 这些没有必要全部用 C++。

⸻

方案 4：全部微服务

比如：

Device Service
SIP Service
Stream Service
PTZ Service
Catalog Service
Record Service
...

最后几十个服务互相 RPC。

对于视频平台尤其危险。

我的建议是：

控制面适当微服务化，媒体面采用 Cell + Worker，而不是无限拆微服务。

⸻

二十六、我认为最合理的系统分层

最终我会把系统划成：

                   ┌───────────────────────┐
                   │      Portal/UI         │
                   └───────────┬───────────┘
                               │
                   ┌───────────▼───────────┐
                   │     API Gateway        │
                   └───────────┬───────────┘
                               │
              ╔════════════════╧════════════════╗
              ║          Control Plane          ║
              ║                                 ║
              ║ Device   Stream   Record   OPS  ║
              ║ Auth     PTZ      Alarm    IAM  ║
              ╚════════════════╤════════════════╝
                               │
                         Event Bus
                               │
              ╔════════════════╧════════════════╗
              ║          Access Plane           ║
              ║                                 ║
              ║ SIP Edge / GB28181 Gateway      ║
              ║ RTSP / ONVIF / Other Adapter    ║
              ╚════════════════╤════════════════╝
                               │
              ╔════════════════╧════════════════╗
              ║           Media Plane           ║
              ║                                 ║
              ║ ZLMediaKit / Media Worker       ║
              ║ RTP / PS / RTSP / WebRTC        ║
              ║ Record / Relay / Transcode      ║
              ╚════════════════╤════════════════╝
                               │
                         Camera Network

这是我比较推荐的产品级结构。

⸻

二十七、如果让我带团队做，我会这样拆开发团队

不是所有人都去写 28181。

团队 A：设备协议

Go
GB28181
SIP
ONVIF
RTSP
设备控制
PTZ
目录

团队 B：媒体

C++
ZLMediaKit
RTP
RTSP
WebRTC
FFmpeg
转码
录像

团队 C：平台

Go
设备管理
权限
组织
租户
录像
报警
API

团队 D：基础设施

Kubernetes
Kafka
Redis
PostgreSQL
监控
日志
CI/CD

团队 E：前端

Vue
TypeScript
WebRTC
地图
设备树
视频墙
PTZ
运维

⸻

二十八、第一阶段不要直接做 100 万

我建议用容量分阶段设计：

Phase 1
1万设备

验证：

28181
注册
KeepAlive
INVITE
RTP
PTZ
WebRTC
录像

↓

Phase 2
10万设备

验证：

Cell
Redis Cluster
Kafka
媒体集群
故障恢复

↓

Phase 3
50万设备

重点验证：

SIP TPS
大量上下线
设备风暴
MediaServer故障
网络抖动
Redis故障
Kafka堆积

↓

Phase 4
100万设备

最终验证：

百万级 REGISTER
百万 KeepAlive
几十万 RTP
十万级观看
海量报警
跨节点调度
全链路故障恢复

⸻

二十九、还有一个非常关键的设计：不要让“设备接入”和“视频拉流”绑定

例如设备在线：

Device Online

不代表：

Stream Active

建议：

Device
   │
   ├── Online
   │
   ├── SIP Session
   │
   └── Stream Session
             │
             └── MediaServer

用户第一次点击：

播放

才：

StartStream
   ↓
INVITE
   ↓
RTP
   ↓
MediaServer

用户停止：

StopStream
   ↓
BYE

这样对于百万设备场景，资源成本会下降非常明显。

⸻

三十、最终我给你的技术选型

如果让我现在开始做，我会直接定：

Go + C++ 混合架构。

核心组合：

Go
│
├── Device Management
├── GB28181 Business
├── SIP Business
├── PTZ
├── Catalog
├── Alarm
├── Record
├── OPS
└── API
C++
│
├── ZLMediaKit
├── RTP
├── RTSP
├── WebRTC
├── Media Relay
├── Record
└── Transcoding
Infrastructure
│
├── Kamailio / OpenSIPS
├── Redis Cluster
├── Kafka
├── PostgreSQL
├── ClickHouse
├── Prometheus
├── MinIO/Ceph
└── Kubernetes

其中我认为最值得坚持的三个原则是：

第一，控制面与媒体面分离。

第二，协议适配层与业务层分离。

第三，100 万设备采用 Cell/Shard，而不是做一个“超级单体集群”。

⸻

如果这是你准备真正立项做的产品，我下一步最建议直接进入**“百万设备视频接入平台详细架构设计”**：把 GB28181 信令流程、SIP Gateway、MediaServer、设备/通道/流/会话数据模型、Cell 分片方式、100 万设备容量计算、服务器规格、网络带宽、Redis/PostgreSQL/Kafka 集群规模，以及 Go/C++ 代码仓库怎么拆一次性设计出来。这样基本就可以直接拿去做技术方案和开发任务拆分。

是的，而且我建议你把“面向用户的平台能力”和“设备/媒体能力”明确拆开。对于百万级视频接入平台，最容易犯的错误就是把“用户、设备、视频、权限、组织、运维”全部放在一个设备管理服务里，前期开发快，后期会非常难扩展。

我建议把整个产品定义成：

统一身份与权限中心 + 资源管理中心 + 视频设备中心 + 媒体中心 + 数据中心 + 安全中心 + 运维中心

其中最核心的设计思想是：

用户不直接“拥有设备”，用户通过“组织/租户 → 资源 → 权限”间接访问设备。

⸻

一、我建议重新划分整个系统

可以把平台分成 7 个一级域：

┌──────────────────────────────────────────────────────────┐
│                    视频接入平台                           │
│                                                          │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐               │
│  │ 用户与权限 │  │ 资源管理 │  │ 安全管理 │               │
│  │ IAM      │  │ Resource │  │ Security │               │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘               │
│       │              │             │                     │
│  ┌────▼─────┐  ┌─────▼─────┐ ┌─────▼─────┐              │
│  │ 设备管理  │  │ 视频媒体   │ │ 数据管理   │              │
│  │ Device   │  │ Media     │ │ Data      │              │
│  └────┬─────┘  └─────┬─────┘ └───────────┘              │
│       │              │                                   │
│       └──────────────┼──────────────────┐                │
│                      ▼                  │                │
│                ┌─────────────┐          │                │
│                │   运维管理   │◄─────────┘                │
│                │    OPS      │                           │
│                └─────────────┘                           │
└──────────────────────────────────────────────────────────┘

但这 7 个域之间不是平级关系。

真正的核心依赖应该是：

IAM
 ↓
组织/资源
 ↓
设备
 ↓
通道
 ↓
流
 ↓
媒体会话

⸻

二、最重要的是建立统一的 Resource 模型

我建议你从项目第一天就设计一个：

Resource（资源）中心

而不是让权限系统直接理解“摄像头”。

例如平台里面所有东西都抽象成资源：

Resource
├── Organization
├── Device
├── Channel
├── Stream
├── Video
├── Record
├── Alarm
├── Map
├── Operation
└── API

但是实际权限控制的核心资源主要是：

组织
设备
通道
录像

例如：

广东省
└── 深圳市
    ├── 南山区
    │   ├── 摄像头001
    │   │   ├── 通道01
    │   │   └── 通道02
    │   │
    │   └── 摄像头002
    │
    └── 福田区
        └── 摄像头003

用户看到的：

深圳市
 └── 南山区
      └── 摄像头001

实际上就是一个：

Resource Tree

⸻

三、不要把“用户”和“设备”直接关联

这是非常重要的一点。

不建议：

User
  ↓
Device

而应该：

User
 ↓
Tenant
 ↓
Organization
 ↓
Resource
 ↓
Device
 ↓
Channel

例如：

张三
 │
 ▼
XX集团
 │
 ▼
广东分公司
 │
 ▼
深圳区域
 │
 ▼
南山区
 │
 ▼
摄像头001

这样未来才能支持：

* 多租户
* 分公司
* 项目
* 部门
* 外包单位
* 第三方用户
* 临时授权
* 跨组织授权

⸻

四、权限模型建议采用 RBAC + ABAC

如果只是：

用户 → 角色 → 权限

很快就不够用了。

我建议：

RBAC + Resource ACL + ABAC

三者结合。

⸻

1. RBAC

解决：

“这个人是什么角色？”

例如：

超级管理员
平台管理员
区域管理员
运维人员
普通用户
监控人员
审计人员

角色拥有：

设备查看
设备控制
实时视频
录像查看
录像下载
PTZ控制
设备配置
报警处理

⸻

五、但是角色不能决定“看哪些摄像头”

例如：

张三 = 监控人员

并不能说明张三可以看：

全部100万摄像头

所以需要：

Role
 +
Resource Scope

例如：

张三
角色：监控人员
资源范围：
广东省

于是：

广东省
 ├── 广州
 ├── 深圳
 ├── 东莞
 └── 珠海

都可以看。

但：

北京
上海
四川

不可以看。

⸻

六、所以权限模型建议这样设计

User
 │
 ├── Roles
 │      │
 │      └── Permissions
 │
 └── ResourceScopes
        │
        └── Resource Tree

最终权限判断：

User
 ↓
是否拥有 Permission？
 ↓
是否匹配 Resource Scope？
 ↓
是否满足 ABAC 条件？
 ↓
Allow / Deny

⸻

七、权限最好拆成“功能权限”和“数据权限”

这是视频平台非常重要的一点。

例如：

功能权限

device:view
device:create
device:update
device:delete
channel:view
stream:play
ptz:control
record:view
record:download
alarm:view
alarm:handle
user:create
user:update
system:config

⸻

数据权限

比如：

device_scope:
    广东省/深圳市/南山区

然后：

张三
拥有：
stream:play
ptz:control
record:view
数据范围：
广东省/深圳市

所以张三：

深圳摄像头：
播放       ✓
PTZ        ✓
录像       ✓
广州摄像头：
播放       ✗
深圳其他区域：
播放       ✓

⸻

八、还有一个容易忽略的权限：操作级权限

视频平台不能只有：

看 / 不看

还应该有：

查看
播放
控制
配置
删除
下载
导出
分享
授权

例如：

实时视频
 ├── 查看
 ├── 播放
 ├── 截图
 ├── 录像
 └── 分享
PTZ
 ├── 查看
 ├── 左右
 ├── 上下
 ├── 变焦
 └── 预置位
录像
 ├── 查询
 ├── 播放
 ├── 下载
 └── 删除

⸻

九、尤其要控制“视频下载”权限

因为：

实时观看权限 ≠ 视频下载权限

例如：

普通监控员
实时播放 ✓
录像播放 ✓
录像下载 ✗

管理员：

实时播放 ✓
录像播放 ✓
录像下载 ✓

这属于典型的数据安全控制。

⸻

十、我建议单独建立 Security Center

不要把安全管理塞到用户管理里面。

建议：

Security Center

至少包含：

身份安全

密码策略
MFA
登录策略
验证码
登录失败锁定
Session
Token
设备登录

访问安全

IP白名单
IP黑名单
访问频率限制
API限流
异常访问
异地登录
并发Session

数据安全

视频访问权限
录像下载权限
水印
敏感数据
数据脱敏
文件访问控制

审计

登录日志
设备操作日志
视频观看日志
录像下载日志
PTZ操作日志
权限变更日志
管理员操作日志

⸻

十一、视频观看本身也必须纳入权限体系

这个设计非常关键。

用户点击：

播放摄像头001

不要直接：

GET /stream/xxx

而应该：

User
 ↓
API Gateway
 ↓
Authorization Service
 ↓
检查：
    stream:play
    resource:channel001
 ↓
Allow
 ↓
Stream Service
 ↓
生成短期播放Token
 ↓
Media Server
 ↓
WebRTC

也就是说：

MediaServer 本身不负责理解“张三有没有权限”。

它只负责：

“这个 token 是否有效？”

⸻

十二、推荐设计一个 Media Access Token

例如：

POST /api/v1/channels/{channelId}/play

返回：

{
  "streamId": "xxx",
  "protocol": "webrtc",
  "url": "...",
  "token": "xxxx",
  "expireAt": 1780000000
}

Token：

TTL = 30s / 60s / 5min

里面可以带：

userId
tenantId
channelId
permission
expireAt
sessionId

这样就不会出现：

用户退出平台以后，之前拿到的播放 URL 还能永久访问。

⸻

十三、设备中心和权限中心怎么关联？

我建议：

             IAM
              │
              ▼
        Resource Center
              │
       ┌──────┴───────┐
       ▼              ▼
 Organization       Device
                      │
                      ▼
                   Channel
                      │
                      ▼
                    Stream

设备创建的时候：

Device Service
     ↓
创建 Device
     ↓
Resource Center
     ↓
生成 Resource

例如：

resource_id:
r-100001
resource_type:
channel
resource_parent:
r-100000
resource_name:
深圳南山摄像头001

权限系统只认：

resource_id

不需要知道这个资源到底来自：

GB28181
RTSP
ONVIF
海康
大华

这就把业务权限和协议解耦了。

⸻

十四、这个设计还能解决一个很大的问题：协议扩展

未来：

GB28181
RTSP
ONVIF
海康SDK
大华SDK
其他厂商

统一变成：

Device
 ↓
Channel
 ↓
Resource

例如：

海康摄像头
       ↓
Hikvision Adapter
       ↓
Device
       ↓
Channel
       ↓
Resource

权限系统完全不需要改变。

⸻

十五、建议增加“项目/空间”这个概念

如果你的平台未来不仅是一个企业内部系统，而是一个大型视频平台，我强烈建议设计：

Tenant
  ↓
Project
  ↓
Organization
  ↓
Resource

例如：

XX集团
│
├── 项目A：智慧园区
│   ├── 园区1
│   └── 园区2
│
├── 项目B：智慧交通
│   ├── 高速公路
│   └── 城市道路
│
└── 项目C：智慧校园

因为现实中：

一个用户可能同时参与多个项目。

如果只有：

User → Organization

后面会很难处理。

⸻

十六、我甚至建议把“资源树”和“设备物理拓扑”分开

这个设计很有价值。

例如物理上：

SIP Gateway
   ↓
Device
   ↓
Channel
   ↓
MediaServer

这是：

技术拓扑

但用户看到的可能是：

广东省
 └── 深圳市
      └── 南山区
           └── XX大厦
                └── 1楼
                     └── 电梯

这是：

业务资源树

二者不要混为一谈。

⸻

十七、所以设备应该有两个“归属”

例如：

Device
│
├── Technical Topology
│     ├── Gateway
│     ├── MediaServer
│     └── Network
│
└── Business Resource
      ├── Tenant
      ├── Project
      ├── Organization
      └── ResourceTree

这会让你的系统非常灵活。

⸻

十八、数据管理也建议独立出来

我会把：

Data Center / Data Service

单独设计。

它负责：

设备数据
通道数据
录像索引
报警数据
操作日志
审计数据
统计数据
运行数据

但是这里需要特别注意：

业务数据和媒体数据分离。

例如：

PostgreSQL
    │
    ├── Device
    ├── Channel
    ├── User
    ├── Organization
    └── Permission
ClickHouse
    │
    ├── PlayLog
    ├── DownloadLog
    ├── AlarmHistory
    ├── DeviceMetrics
    └── OperationLog
Object Storage
    │
    ├── Video
    ├── Snapshot
    └── Recording

⸻

十九、我建议增加“审计中心”

对于百万级视频平台，这个东西非常重要。

尤其涉及：

谁看过哪个摄像头？
谁下载过录像？
谁控制过云台？
谁修改过设备？
谁授权了谁？
谁删除了录像？
谁修改了权限？

例如：

2026-08-14 20:15:31
张三
播放
深圳南山
摄像头001
成功

以及：

2026-08-14 20:17:05
张三
下载录像
摄像头001
19:00-19:10
成功

这个日志最好做到：

不可篡改 + 可追溯 + 可审计。

⸻

二十、最终的业务架构，我建议调整成这样

                           用户 / 第三方系统
                                  │
                                  ▼
                         ┌─────────────────┐
                         │   API Gateway   │
                         └────────┬────────┘
                                  │
        ┌─────────────────────────┼────────────────────────┐
        │                         │                        │
        ▼                         ▼                        ▼
┌──────────────┐         ┌──────────────┐         ┌──────────────┐
│ IAM / 权限中心 │         │ 资源管理中心  │         │ 安全中心      │
│              │         │              │         │              │
│ User         │         │ Tenant       │         │ MFA          │
│ Role         │         │ Project      │         │ Token        │
│ Permission   │         │ Organization │         │ IP Policy    │
│ Policy       │         │ Resource     │         │ Audit        │
└──────┬───────┘         └──────┬───────┘         └──────┬───────┘
       │                        │                        │
       └────────────────────────┼────────────────────────┘
                                │
                                ▼
                       ┌─────────────────┐
                       │   Device Center │
                       │                 │
                       │ Device          │
                       │ Channel         │
                       │ Protocol        │
                       │ PTZ             │
                       │ Alarm           │
                       └────────┬────────┘
                                │
                ┌───────────────┴────────────────┐
                ▼                                ▼
       ┌─────────────────┐              ┌─────────────────┐
       │ Stream Manager  │              │ Record Manager  │
       └────────┬────────┘              └────────┬────────┘
                │                                │
                ▼                                ▼
       ┌─────────────────┐              ┌─────────────────┐
       │ Media Cluster   │              │ Object Storage  │
       │ ZLMediaKit      │              │ MinIO / Ceph    │
       └─────────────────┘              └─────────────────┘
                        ┌─────────────────┐
                        │   OPS Center    │
                        │                 │
                        │ Device Health   │
                        │ Media Health    │
                        │ SIP Health      │
                        │ Network         │
                        │ Alert           │
                        └─────────────────┘

⸻

二十一、这里还有一个非常重要的边界

我建议你把：

IAM

负责：

“你是谁？”

Authorization

负责：

“你能干什么？”

Resource

负责：

“你能访问什么？”

Device

负责：

“设备是什么？”

Stream

负责：

“视频在哪里？”

Media

负责：

“视频怎么传？”

Security

负责：

“访问是否安全？”

Audit

负责：

“谁做了什么？”

这 8 个问题彻底分开。

这样整个系统会非常干净。

⸻

二十二、我会特别建议你做一个统一的 Authorization API

所有模块都不要自己判断权限。

统一：

Authorization Service

例如：

POST /api/v1/authz/check

请求：

{
  "subject": "user-10001",
  "action": "stream:play",
  "resource": "channel-20001"
}

返回：

{
  "allowed": true
}

或者批量：

{
  "subject": "user-10001",
  "actions": [
    "channel:view",
    "stream:play",
    "ptz:control"
  ],
  "resources": [
    "channel-20001",
    "channel-20002"
  ]
}

这样：

设备服务
媒体服务
录像服务
报警服务
API

都使用统一授权逻辑。

⸻

二十三、但不要每次播放都查数据库

百万设备下，这会成为灾难。

应该：

                  Authorization
                        │
                        ▼
                  Policy Engine
                        │
                        ▼
                      Redis
                        │
                Permission Cache

用户登录后：

User
 ↓
Role
 ↓
Permission
 ↓
Resource Scope
 ↓
生成权限缓存

播放时：

Play Request
     ↓
Gateway
     ↓
Auth Cache
     ↓
几十微秒～毫秒级判断

而不是：

Play
 ↓
PostgreSQL
 ↓
Role
 ↓
Permission
 ↓
Resource
 ↓
Organization
 ↓
返回

⸻

二十四、如果你准备把这个产品真正做成“平台”，我建议最终形成下面这套产品模块

一级模块    二级模块
工作台    首页、视频总览、告警总览、设备总览
设备中心    设备、通道、国标、协议、PTZ、设备配置
视频中心    实时视频、视频墙、录像、回放、云台
资源中心    租户、项目、组织、资源树、区域
权限中心    用户、角色、权限、数据权限、授权
数据中心    设备数据、录像索引、统计、报表
安全中心    登录安全、访问控制、MFA、Token、审计
运维中心    设备运维、媒体运维、SIP、网络、告警
开放平台    OpenAPI、Webhook、SDK、第三方接入
系统管理    参数、字典、配置、集群、节点
审计中心    登录、观看、下载、控制、配置、权限变更

这样，它就不再只是一个：

“GB28181 视频接入服务器”

而是一个完整的：

“百万级视频物联接入与管理平台”。

⸻

我认为下一步应该重点做一件事

你现在已经从“采用什么技术”进入到“产品+技术总体架构”阶段了。

下一步我建议我们直接把这个系统做成一张完整的 L0～L3 架构图 + 模块职责矩阵，重点把下面几个东西定死：

1. 租户 Tenant
2. 项目 Project
3. 组织 Organization
4. 用户 User
5. 角色 Role
6. 权限 Permission
7. 资源 Resource
8. 设备 Device
9. 通道 Channel
10. 流 Stream
11. 媒体会话 Session
12. 录像 Record
13. 告警 Alarm
14. 审计 Audit
15. 运维 OPS

尤其要把**“用户 → 租户 → 项目 → 组织 → 资源 → 设备 → 通道 → 流 → MediaServer”**这一整条关系画清楚，再进一步设计数据库表和服务边界。

这一步一旦设计正确，后面的 Go 服务怎么拆、Redis 怎么存、PostgreSQL 表怎么建、Kafka Topic 怎么划、100 万设备怎么分片、WebRTC 播放权限怎么控制都会顺下来。

将以上关于我和 ChatGPT 讨论这个项目设计、架构设计、技术选型、模块划分等技术细节记录下来，然后帮我整理一个比较详细、完整的项目设计方案出来。