package flag

// Flags 是命令行指令的集合(解析结果)。当一个字段为 true、非空指针或有效字符串时,指令有效。
type Flags struct {
	run           bool     //启动服务
	help          bool     //显示帮助
	pluginDetails bool     //显示各组件描述(DESC)
	ConfigPath    string   //配置文件路径;为空时用 DefaultSettingPath
	InitDB        *initDB  //初始化数据库
	Setting       *Setting //配置的复制与输出
}

// initDB 初始化数据库指令,内部操作来确认初始化哪些数据库。
type initDB struct {
	All  bool //初始化全部
	Blog bool
}

// Type 是配置输出格式。
type Type int8

const (
	Yaml Type = iota + 1 // 1:yaml
	Json                 // 2:json
)

// String 返回格式名。
func (t Type) String() string {
	if t == Json {
		return "json"
	}
	return "yaml"
}

// Setting 描述配置的初始化 / 拷贝指令。
type Setting struct {
	InitSetting string //输出默认配置到路径
	CopySetting string //拷贝现有配置到指定路径
	Type        Type   //输出格式
}

// waitTask 是一个"后功能待办":等 component 上线后执行 run。
type waitTask struct {
	topic     string
	component string
	run       func()
}
