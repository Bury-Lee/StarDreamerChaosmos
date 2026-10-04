package common

// BootService 是宿主预挂的「启动选项」槽位名。
// 宿主在启用业务组件之前写入,组件在自己的 Apply(上线)阶段读取,
// 据此决定是否执行数据库迁移等一次性动作。
const BootService = "host/boot"

// Boot 是启动选项:由宿主按命令行指令(如 initdb)填好,组件上线时读取。
type Boot struct {
	InitDB bool // 是否要求初始化数据库(执行模型迁移)
}
