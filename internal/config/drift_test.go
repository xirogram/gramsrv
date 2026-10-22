package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// writeDriftFiles 在临时目录落一对 .env / .env.example 并返回路径。省略的内容
// 代表文件不存在。
func writeDriftFiles(t *testing.T, example, env string) (cfgPath, examplePath string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath = filepath.Join(dir, ".env")
	examplePath = cfgPath + ".example"
	if example != "" {
		if err := os.WriteFile(examplePath, []byte(example), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if env != "" {
		if err := os.WriteFile(cfgPath, []byte(env), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return cfgPath, examplePath
}

func TestDetectEnvDriftSplitsMissingAndUnknown(t *testing.T) {
	cfgPath, examplePath := writeDriftFiles(t,
		"## Group -- d\nTELESRV_ALPHA=1\n# TELESRV_OPTIONAL=\nTELESRV_BETA=2\n",
		"TELESRV_BETA=custom\nTELESRV_TYPO_KEY=x\n")

	drift, err := envDriftAt(cfgPath, examplePath)
	if err != nil {
		t.Fatalf("envDriftAt: %v", err)
	}
	// ALPHA 与被注释的 OPTIONAL 都声明在模板里、本机未设 → Missing，且保持模板顺序。
	if want := []string{"TELESRV_ALPHA", "TELESRV_OPTIONAL"}; !reflect.DeepEqual(drift.Missing, want) {
		t.Errorf("Missing = %v, want %v", drift.Missing, want)
	}
	// BETA 两边都有；TYPO_KEY 只在 .env → Unknown（笔误信号）。
	if want := []string{"TELESRV_TYPO_KEY"}; !reflect.DeepEqual(drift.Unknown, want) {
		t.Errorf("Unknown = %v, want %v", drift.Unknown, want)
	}
}

func TestDetectEnvDriftProcessEnvCountsAsSet(t *testing.T) {
	cfgPath, examplePath := writeDriftFiles(t,
		"TELESRV_ALPHA=\nTELESRV_BETA=\n",
		"")
	// 进程环境优先级高于文件：设了就不算缺失，哪怕 .env 里没有。
	t.Setenv("TELESRV_ALPHA", "from-process")

	drift, err := envDriftAt(cfgPath, examplePath)
	if err != nil {
		t.Fatalf("envDriftAt: %v", err)
	}
	if want := []string{"TELESRV_BETA"}; !reflect.DeepEqual(drift.Missing, want) {
		t.Errorf("Missing = %v, want %v", drift.Missing, want)
	}
}

func TestDetectEnvDriftMissingTemplateIsNotAnError(t *testing.T) {
	// 生产镜像不带 .env.example：必须静默跳过，而不是启动报错。
	cfgPath, examplePath := writeDriftFiles(t, "", "TELESRV_ALPHA=1\n")

	drift, err := envDriftAt(cfgPath, examplePath)
	if err != nil {
		t.Fatalf("envDriftAt: %v", err)
	}
	if len(drift.Missing) != 0 || len(drift.Unknown) != 0 {
		t.Errorf("drift = %+v, want zero value when template is absent", drift)
	}
}

func TestDetectEnvDriftWithoutConfigFileListsTemplateDefaults(t *testing.T) {
	// 没有 .env（配置全走进程环境/默认值）时，模板里每个键都进入 Missing，
	// 由调用方按 INFO 展示——这正是「新装机/纯环境变量部署」的可见性。
	cfgPath, examplePath := writeDriftFiles(t,
		"TELESRV_ALPHA=1\nTELESRV_BETA=2\n",
		"")

	drift, err := envDriftAt(cfgPath, examplePath)
	if err != nil {
		t.Fatalf("envDriftAt: %v", err)
	}
	if want := []string{"TELESRV_ALPHA", "TELESRV_BETA"}; !reflect.DeepEqual(drift.Missing, want) {
		t.Errorf("Missing = %v, want %v", drift.Missing, want)
	}
	if len(drift.Unknown) != 0 {
		t.Errorf("Unknown = %v, want none", drift.Unknown)
	}
}

func TestDetectEnvDriftUnknownKeysAreSorted(t *testing.T) {
	// .env 由 map 读入、遍历顺序随机，报告必须稳定（日志与断言都依赖它）。
	cfgPath, examplePath := writeDriftFiles(t,
		"TELESRV_KEEP=1\n",
		"TELESRV_ZULU=1\nTELESRV_ALFA=1\nTELESRV_KEEP=2\n")

	drift, err := envDriftAt(cfgPath, examplePath)
	if err != nil {
		t.Fatalf("envDriftAt: %v", err)
	}
	if want := []string{"TELESRV_ALFA", "TELESRV_ZULU"}; !reflect.DeepEqual(drift.Unknown, want) {
		t.Errorf("Unknown = %v, want %v", drift.Unknown, want)
	}
}

func TestDetectEnvDriftIgnoresNonFieldLines(t *testing.T) {
	// 模板里有分组头、说明注释与非 TELESRV_ 行，都不能被当成字段键。
	cfgPath, examplePath := writeDriftFiles(t,
		"## Server & Network -- Where the server listens\n"+
			"# это просто комментарий без ключа\n"+
			"# =====\n"+
			"POSTGRES_PASSWORD=irrelevant\n"+
			"TELESRV_LISTEN=127.0.0.1:2398\n",
		"TELESRV_LISTEN=127.0.0.1:2398\n")

	drift, err := envDriftAt(cfgPath, examplePath)
	if err != nil {
		t.Fatalf("envDriftAt: %v", err)
	}
	if len(drift.Missing) != 0 || len(drift.Unknown) != 0 {
		t.Errorf("drift = %+v, want no drift for pure documentation lines", drift)
	}
}

func TestDetectEnvDriftAcceptsConfigKeySyntaxWithoutLoggingValues(t *testing.T) {
	cfgPath, examplePath := writeDriftFiles(t,
		"export TELESRV_ALPHA = a-secret\n# export TELESRV_OPTIONAL = example-secret\nTELESRV_BETA=template-secret\n",
		"export TELESRV_ALPHA = actual-secret\nTELESRV_TYPO=another-secret\n")
	drift, err := envDriftAt(cfgPath, examplePath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(drift.Missing, []string{"TELESRV_OPTIONAL", "TELESRV_BETA"}) ||
		!reflect.DeepEqual(drift.Unknown, []string{"TELESRV_TYPO"}) {
		t.Fatalf("unexpected drift: %+v", drift)
	}
}

// TestRepoEnvExampleParses 验证仓库根真实 .env.example 能被漂移比对完整解析：
// 分组头、说明注释、占位行都不得被当成字段，且必须解析出文档规模的一批字段键。
// （方向「模板里有、代码没人读」涉及动态键与外置二进制，不在此守。）
func TestRepoEnvExampleParses(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	keys, err := readExampleKeys(filepath.Join(repoRoot, ".env.example"))
	if err != nil {
		t.Fatalf("readExampleKeys: %v", err)
	}
	if len(keys) < 100 {
		t.Fatalf("parsed %d keys from .env.example, want the documented hundreds", len(keys))
	}
	seen := map[string]bool{}
	for _, k := range keys {
		if seen[k] {
			t.Errorf("duplicate key %s in .env.example", k)
		}
		seen[k] = true
	}
}
