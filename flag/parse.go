package flag

import (
	"fmt"
	"strings"

	"StarDreamerChaosmos/utils"
)

// Parse 解析命令行参数;argv 含程序名。
//
// 支持的指令:
//
//	run                          启动服务
//	initdb [all|blog]            初始化数据库(缺省 all)
//	init-setting <path>          输出默认配置到路径
//	copy-setting <path>          拷贝现有配置到路径
//	-config <path>               指定配置文件路径
//	-type <yaml|json>            配置输出格式(缺省 yaml)
func Parse(argv []string) (*Flags, error) {
	f := &Flags{}
	args := argv
	if len(args) > 0 {
		args = args[1:] // 跳过程序名
	}
	var setting *Setting
	var setType Type

	for i := 0; i < len(args); i++ {
		a := args[i]
		name, val, hasVal := utils.SplitFlag(a)
		if name == "" {
			name = a // 位置指令(无 - 前缀)
		}
		switch name {
		case "run":
			f.run = true
		case "help", "h":
			f.help = true
		case "plugin":
			sub := "details"
			if hasVal {
				sub = val
			} else if i+1 < len(args) && !utils.IsFlag(args[i+1]) {
				sub = args[i+1]
				i++
			}
			switch sub {
			case "details":
				f.pluginDetails = true
			default:
				return nil, fmt.Errorf("未知的 plugin 子命令 %q(期望 details)", sub)
			}
		case "initdb":
			sub := "all"
			if i+1 < len(args) && !utils.IsFlag(args[i+1]) {
				sub = args[i+1]
				i++
			}
			d := &initDB{}
			switch sub {
			case "all":
				d.All = true
			case "blog":
				d.Blog = true
			default:
				return nil, fmt.Errorf("未知的 initdb 子命令 %q(期望 all|blog)", sub)
			}
			f.InitDB = d
		case "init-setting", "copy-setting":
			path, ni, err := utils.NextValue(args, i, name, val, hasVal)
			if err != nil {
				return nil, err
			}
			i = ni
			if setting == nil {
				setting = &Setting{}
			}
			if name == "init-setting" {
				setting.InitSetting = path
			} else {
				setting.CopySetting = path
			}
		case "type", "t":
			v, ni, err := utils.NextValue(args, i, name, val, hasVal)
			if err != nil {
				return nil, err
			}
			i = ni
			tt, err := parseType(v)
			if err != nil {
				return nil, err
			}
			setType = tt
		case "config", "c":
			v, ni, err := utils.NextValue(args, i, name, val, hasVal)
			if err != nil {
				return nil, err
			}
			i = ni
			f.ConfigPath = v
		default:
			return nil, fmt.Errorf("未知指令 %q", a)
		}
	}
	if setting != nil {
		if setType == 0 {
			setType = Yaml
		}
		setting.Type = setType
		f.Setting = setting
	}
	return f, nil
}

// parseType 解析输出格式。
func parseType(s string) (Type, error) {
	switch strings.ToLower(s) {
	case "yaml", "yml":
		return Yaml, nil
	case "json":
		return Json, nil
	default:
		return 0, fmt.Errorf("未知输出格式 %q(期望 yaml|json)", s)
	}
}
