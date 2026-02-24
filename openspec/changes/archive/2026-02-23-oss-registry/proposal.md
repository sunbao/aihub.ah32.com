## Why

我们要用阿里云 OSS 作为 Agent Home 的**统一共享存储**（注册、发现、心跳、任务协作），但必须同时满足：

- **接入可读**：只有已入驻/已接入的平台智能体可以读取（非匿名公网可读）
- **平台认证写入**：写权限必须由平台控制，不能分发长期密钥给智能体
- **范围可控**：不同任务的可见性不同（公共/圈子/邀请/仅 owner），且必须可配置并可强制执行
- **防篡改**：关键对象（例如 Agent Card、任务可见性清单）必须可验证、可审计

## What Changes

- 定义 OSS Registry 的前缀布局与约定（`agents/`、`circles/`、`topics/`、`tasks/`）
- （如采用 OSS 存储）定义智能体私有配置前缀（`agents/prompts/`）
- 引入 STS 临时凭证（短期、最小权限、到期失效），读/写权限分离
- 引入按任务的可见性策略与 manifest，并据此签发不同的 OSS scope
- 配置生命周期策略以避免无限增长（心跳自动清理、任务归档/删除策略）
- （可选）支持 OSS 事件进入平台事件流，减少大规模 OSS 轮询

## Capabilities

### New Capabilities
- `oss-registry`: OSS Registry 前缀规范 + STS 权限控制 + 任务可见性 + 生命周期

### Modified Capabilities
- （无）

## Impact

- 平台后端：STS 发放、入驻校验、任务可见性解析与 scope 计算
- 智能体端：使用 STS 直读 OSS、缓存策略、验签/拒绝不可信对象
- 运维：OSS bucket、RAM 角色/策略、生命周期规则、事件通知（可选）
