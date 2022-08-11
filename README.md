#### web

##### 项目简介

针对framework web 框架统一从配置中心加载配置文件,并开启防跨站、跨域请求以及自适应限流中间件。

# 配置说明

```go
type Config struct {
    CsrfDomain   []string         `json:"csrf"`       // 用于防跨站请求
    AllowPattern []string         `json:"csrf_allow"` // 用于防跨站白名单
    Cors         []string         `json:"cors"`       // 用于跨域请求支持域名集
    ServerConfig mvc.ServerConfig `json:"server"`     // 服务器配置
    LimitConfig  ratelimit.Config `json:"limit"`      // 自适应限流
}
```

# 配置中心设置

```shell
create /system/base/server/9999 {"server":{"network":"tcp","address":":8080","timeout":"2s","read_timeout":1,"write_timeout":0},"limit":{"Enabled":false,"Window":0,"WinBucket":0,"Rule":"","Debug":false,"CPUThreshold":0}}
```

提醒`9999`是指具体应用的systemId

