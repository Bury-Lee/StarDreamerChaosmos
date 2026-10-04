package flag

import "GoTenon"

//这里放一些硬编码的参数
const DefaultSettingPath = "../Setting.yaml"

// FlagsService 是解析结果(*Flags)的槽位名:由 flag 在自身 Apply 的子树内 Isolate,
// 属组件私有槽位(原始参数改由宿主经 Register 的 cfg 传入,不再占用 root 槽位)。
const FlagsService = "svc/flags"

// TypeSettingRequest 是"请执行配置初始化 / 拷贝"的应用级消息类型:
// flag 经 mq 转发给 config。MQ 占用 TypeCustomBase 起的 16–31,应用级从 +16 开始。
const TypeSettingRequest GoTenon.MessageType = GoTenon.TypeCustomBase + 16

// Usage 是帮助文本(help 指令输出)。
const Usage = `StarDreamerChaosmos 可用命令:
  help                        显示本帮助
  run                         启动服务
  initdb [all|blog]           初始化数据库(缺省 all)
  init-setting <path>         输出默认配置到路径
  copy-setting <path>         拷贝现有配置到路径
  plugin details              显示各组件描述(DESC)
  -config <path>              指定配置文件路径
  -type <yaml|json>           配置输出格式(缺省 yaml)`
