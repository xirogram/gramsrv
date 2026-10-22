package config

// 本文件做 .env 与 .env.example 的启动期比对（配置漂移报告），只读、不写回：
//
//   - Missing（.env.example 有、.env 与进程环境都没有）：这些设置此刻正跑在代码
//     默认值上，多数是刻意的（可选项本来就不填），但「新版本加了配置、.env 还是
//     旧的」这类遗漏也藏在这里，所以只报 INFO。
//   - Unknown（.env 有、.env.example 没有）：几乎必然是键名笔误或模板滞后——
//     readEnvFile 接受任何 TELESRV_* 键名，而 Load 只查询自己认识的键，笔误值
//     会被静默忽略，服务照常带着默认值启动。这类要报 WARN。
//
// 比对在 config.Load 成功之后调用（cmd/telesrv/main.go），因此 .env 本身已通过
// readEnvFile 的校验。绝不自动改写 .env：容器是 read-only rootfs，且生产配置
// 常走进程环境而非文件；静默改动运维的配置文件比漏报更糟。

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

var (
	// driftActiveKeyRe / driftCommentedKeyRe 与 procctl 的字段正则同义：模板里
	// 的字段行要么是激活的 `K=V`，要么是注释掉的可选字段 `# K=V`。
	driftActiveKeyRe    = regexp.MustCompile(`^(?:export\s+)?(TELESRV_[A-Z0-9_]+)\s*=`)
	driftCommentedKeyRe = regexp.MustCompile(`^#\s*(?:export\s+)?(TELESRV_[A-Z0-9_]+)\s*=`)
)

// EnvDrift 是 .env 相对 .env.example 的差异报告，键保持模板/文件各自的出现顺序。
type EnvDrift struct {
	// ConfigPath 是被比对的配置文件（TELESRV_CONFIG 或默认 .env）。
	ConfigPath string
	// ExamplePath 是模板文件路径。
	ExamplePath string
	// Missing 在 .env.example 中声明、但 .env 与进程环境都没给出的键。
	Missing []string
	// Unknown 在 .env 中出现、但 .env.example 未收录的键（笔误或模板滞后）。
	Unknown []string
}

// DetectEnvDrift 按 Load 相同的规则定位配置文件并比对它与 <配置文件>.example。
// 模板不存在时返回零值且不报错——生产镜像不带 .env.example，属预期情况。
func DetectEnvDrift() (EnvDrift, error) {
	cfgPath, _ := configFilePath()
	if cfgPath == "" {
		return EnvDrift{}, nil
	}
	return envDriftAt(cfgPath, cfgPath+".example")
}

func envDriftAt(cfgPath, examplePath string) (EnvDrift, error) {
	drift := EnvDrift{ConfigPath: cfgPath, ExamplePath: examplePath}
	exampleKeys, err := readExampleKeys(examplePath)
	if err != nil {
		if os.IsNotExist(err) {
			return EnvDrift{}, nil
		}
		return EnvDrift{}, fmt.Errorf("read %s: %w", examplePath, err)
	}
	// .env 缺失时按「全部走默认值」处理：此时通常配置全在进程环境里，下面逐键
	// 用 os.LookupEnv 过滤，真正缺的才会进入 Missing。与 loadConfigEnv 一致——
	// 缺省路径的文件不存在不算错误。
	var envValues envSource
	if cfgPath != "" {
		if envValues, err = readEnvFile(cfgPath); err != nil {
			if !os.IsNotExist(err) {
				return EnvDrift{}, fmt.Errorf("read %s: %w", cfgPath, err)
			}
			err = nil
			envValues = nil
		}
	}

	exampleSet := make(map[string]struct{}, len(exampleKeys))
	for _, key := range exampleKeys {
		exampleSet[key] = struct{}{}
		if _, inFile := envValues[key]; inFile {
			continue
		}
		// 进程环境优先级高于文件（envOr 先查 os.Getenv），设置了就不算缺失。
		if _, inEnv := os.LookupEnv(key); inEnv {
			continue
		}
		drift.Missing = append(drift.Missing, key)
	}
	for key := range envValues {
		if _, ok := exampleSet[key]; !ok {
			drift.Unknown = append(drift.Unknown, key)
		}
	}
	// envValues 是 map，遍历顺序随机；排一次让日志与测试都稳定。
	sort.Strings(drift.Unknown)
	return drift, nil
}

// readExampleKeys 按出现顺序取出模板里全部 TELESRV_* 字段键（激活行与被注释的
// 可选字段行都算），并去重；模板里的注释、分组头与占位行一律跳过。
func readExampleKeys(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var keys []string
	seen := make(map[string]struct{})
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		var key string
		if a := driftActiveKeyRe.FindStringSubmatch(line); a != nil {
			key = a[1]
		} else if c := driftCommentedKeyRe.FindStringSubmatch(line); c != nil {
			key = c[1]
		}
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	return keys, nil
}
