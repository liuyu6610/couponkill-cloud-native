package render

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initJob 返回渲染出的 Kafka 主题初始化 Job。
func initJob(t *testing.T, objs []obj) obj {
	t.Helper()
	job := find(objs, "Job", "kafka-topic-init")
	if job == nil {
		t.Fatal("缺少 Job kafka-topic-init")
	}
	return job
}

// TestKafkaTopicInitJob Job 是 post-install/post-upgrade Hook，只创建 seckill_order_create 与 seckill_order_create_dlt，
// 分区数可配置，不再创建 Java 时代的主题；Job Pod 不带网格 sidecar，非 root。
func TestKafkaTopicInitJob(t *testing.T) {
	objs := renderCombo(t, "default")
	job := initJob(t, objs)
	if hook := job.str("metadata", "annotations", "helm.sh/hook"); hook != "post-install,post-upgrade" {
		t.Errorf("Hook=%q", hook)
	}
	_, env := containerEnv(container(job))
	if got := env["KAFKA_TOPICS"].Value; got != "seckill_order_create:16,seckill_order_create_dlt:4" {
		t.Errorf("KAFKA_TOPICS=%q", got)
	}
	if env["KAFKA_BROKER"].Value != "kafka:9092" || env["KAFKA_REPLICATION_FACTOR"].Value != "1" {
		t.Errorf("KAFKA_BROKER/REPLICATION_FACTOR 不对: %+v %+v", env["KAFKA_BROKER"], env["KAFKA_REPLICATION_FACTOR"])
	}
	ps := podSpec(job)
	if ps.str("securityContext", "runAsNonRoot") != "true" {
		t.Error("Job 应以非 root 运行")
	}
	if job.str("spec", "template", "metadata", "annotations", "sidecar.istio.io/inject") != "false" {
		t.Error("Job 不应注入 Istio sidecar（否则永远不会 Complete）")
	}

	custom := renderCombo(t, "default", "--set", "kafka.topics.seckillOrderCreate.partitions=32",
		"--set", "kafka.topics.deadLetter.partitions=2", "--set", "kafka.replicationFactor=3")
	_, env = containerEnv(container(initJob(t, custom)))
	if got := env["KAFKA_TOPICS"].Value; got != "seckill_order_create:32,seckill_order_create_dlt:2" {
		t.Errorf("自定义分区后 KAFKA_TOPICS=%q", got)
	}
	if env["KAFKA_REPLICATION_FACTOR"].Value != "3" {
		t.Errorf("复制因子未生效: %q", env["KAFKA_REPLICATION_FACTOR"].Value)
	}

	if find(renderCombo(t, "bare"), "Job", "kafka-topic-init") != nil {
		t.Error("kafka.topicInit.enabled=false 时不应渲染 Job")
	}
	if find(renderCombo(t, "external-deps"), "Job", "kafka-topic-init") == nil {
		t.Error("外部 Kafka 也应由 Job 建主题")
	}
	_, env = containerEnv(container(initJob(t, renderCombo(t, "external-deps"))))
	if env["KAFKA_BROKER"].Value != "k1:9092,k2:9092" {
		t.Errorf("外部 Kafka 的 KAFKA_BROKER=%q", env["KAFKA_BROKER"].Value)
	}
}

// fakeKafkaTopics 是假的 kafka-topics.sh：把每次调用记到 $KT_LOG；
// 主题 $FAIL_TOPIC 的 --create 返回失败；--alter 记录后返回 $ALTER_RC（默认 0）。
const fakeKafkaTopics = `#!/bin/bash
echo "$@" >> "$KT_LOG"
for a in "$@"; do
  if [ "$a" = "--list" ]; then exit 0; fi
done
action=""; topic=""; prev=""
for a in "$@"; do
  case "$a" in --create|--alter) action="$a";; esac
  if [ "$prev" = "--topic" ]; then topic="$a"; fi
  prev="$a"
done
if [ "$action" = "--create" ] && [ "$topic" = "${FAIL_TOPIC:-}" ]; then exit 1; fi
if [ "$action" = "--alter" ]; then exit "${ALTER_RC:-0}"; fi
exit 0
`

// runInitScript 用假的 kafka-topics.sh 执行 Job 渲染出的脚本，返回调用日志与退出错误。
func runInitScript(t *testing.T, objs []obj, extraEnv ...string) (string, error) {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("没有 bash，跳过 Job 脚本行为测试")
	}
	if strings.Contains(strings.ToLower(bash), "system32") {
		t.Skip("PATH 上的 bash 是 WSL 启动器而不是 Git Bash，跳过 Job 脚本行为测试")
	}
	job := initJob(t, objs)
	cnt := container(job)
	script := cnt.str("command", 2)
	if script == "" {
		t.Fatal("Job 没有内联脚本")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "kafka-topics.sh")
	if err := os.WriteFile(fake, []byte(fakeKafkaTopics), 0o755); err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(dir, "calls.log")
	_, env := containerEnv(cnt)
	cmd := exec.Command(bash, "-c", script)
	cmd.Env = append(os.Environ(),
		"KAFKA_BROKER="+env["KAFKA_BROKER"].Value,
		"KAFKA_TOPICS="+env["KAFKA_TOPICS"].Value,
		"KAFKA_REPLICATION_FACTOR="+env["KAFKA_REPLICATION_FACTOR"].Value,
		"KAFKA_TOPICS_BIN="+filepath.ToSlash(fake),
		"KAFKA_WAIT_TRIES=2", "KAFKA_WAIT_SLEEP=0",
		"KT_LOG="+filepath.ToSlash(logFile),
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	out, runErr := cmd.CombinedOutput()
	logged, _ := os.ReadFile(logFile)
	t.Logf("脚本输出:\n%s", out)
	return string(logged), runErr
}

// TestKafkaTopicInitScript 脚本创建两个主题（并把已存在主题的分区数调到期望值）；某个主题创建失败时非 0 退出让 Job 重试；
// 调高分区被 broker 拒绝（减少分区）不算失败。
func TestKafkaTopicInitScript(t *testing.T) {
	objs := renderCombo(t, "default")

	calls, err := runInitScript(t, objs)
	if err != nil {
		t.Fatalf("脚本应成功: %v", err)
	}
	for _, want := range []string{
		"--create --if-not-exists --topic seckill_order_create --partitions 16 --replication-factor 1",
		"--create --if-not-exists --topic seckill_order_create_dlt --partitions 4 --replication-factor 1",
		"--alter --topic seckill_order_create --partitions 16",
		"--alter --topic seckill_order_create_dlt --partitions 4",
	} {
		if !strings.Contains(calls, want) {
			t.Errorf("kafka-topics 调用里缺少 %q:\n%s", want, calls)
		}
	}
	if strings.Count(calls, "--create") != 2 {
		t.Errorf("只应创建两个主题:\n%s", calls)
	}

	if _, err := runInitScript(t, objs, "FAIL_TOPIC=seckill_order_create_dlt"); err == nil {
		t.Error("有主题创建失败时脚本应以非 0 退出，让 Job 重试")
	}
	if _, err := runInitScript(t, objs, "ALTER_RC=1"); err != nil {
		t.Errorf("分区数无法调整（已存在且相等/更少）不应让 Job 失败: %v", err)
	}
}
