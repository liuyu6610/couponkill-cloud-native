# 06 · ops-tooling 分片探查报告（OpenSpec explore，只读）

## 0. TL;DR

- 是什么：本分片是 1 套活跃工具加 4 块历史或无关资产。`scripts/*.ps1` 是 Windows 本地联调、冒烟、E2E 工具链（2026-07-12 至 07-18 连续提交，`docs/DEPLOYMENT-SOURCE-OF-TRUTH.md:13-14` 把 `import-nacos-local.ps1` 和 `local-http-smoke.ps1` 列为本地联调口径）；`ansible/` 是已标 DEPRECATED 的阿里云 ECS 通用剧本模板；`jmeter/` 是 2025-09 的秒杀压测计划；`linux-shell-sh/`、`test-python/` 是与 couponkill 无关的学习材料。
- 健康度：scripts 可用，且与后端契约一致（调用的 18 个后端接口全部在 Controller 中定位到）；其余目录要么跑不通，要么与项目无关。本分片没有发现「高」级问题（活跃部分只作用于本机；生产相关的 ansible 路径已废弃，而且是 fail-closed）。
- 最重要的 5 个发现：
  1. `ansible/` 按现状跑不通，也对不上 couponkill：清单脚本 `to_safe()` 把 `tag_env=prod` 生成为 `tag_env_prod`，和 playbook、group_vars 的组名不符；`ansible.cfg` 没有 `roles_path`，`group_vars/` 也不在可加载位置；3 个 tasks 文件内嵌 `handlers:`（非法 YAML）；10 处 `warn:` 参数在 ansible-core 2.14 起已移除；部署模型全是 `yourapp` / 8080 / `/healthz` 占位。嵌套 CI `ansible/ansible/.github/workflows/ci.yml` 不会被 GitHub 触发。
  2. 敏感物：`ansible/ansible/vault/yourapp.yml` 没有用 ansible-vault 加密，但唯一的值是 `community.hashi_vault` lookup 表达式（不是明文口令），且没有任何地方引用；`files/keys/` 只有 1 个 ssh-rsa 公钥（结构非法，推断为占位），没有私钥；4 个 RAM 策略都没有 `Action:*`，但 `AnsibleSlbBackendManage.json` 对 SLB 写操作用了 `Resource:*`，过宽。
  3. fireDue E2E 直连 coupon-service 调 `seckill-window` / `preheat-stock` 能成功，说明券写接口只靠网关鉴权、服务自身没有校验（跨分片观察）；同时这个 E2E 把 `QUEUED` 当 PASS、通知缺失只打 WARN，断言偏弱。
  4. 脚本缺陷：`scripts/local-http-smoke.ps1:79` 清理正则漏掉网关（以及只含主类包名的 fork JVM），推断会遗留 8088 上的旧进程；`local-loadtest.ps1` 不清理用户去重键，重复运行结果失真；`jdk25-aot-train.ps1` 只做 AOT record、不产出 AOTCache，也没有消费者；JVM 参数里有 JDK 24 起已 obsolete 的 `-XX:+ZGenerational`；`_tmp_migrate_user_errorcodes.py` 是已完成的一次性 codemod（未被 git 跟踪，可删）。
  5. jmeter 压测当前无效：CSV 只剩表头（约 3076 条 JWT 在 6b3e9b5 删除，但仍在 git 历史里）；断言找"秒杀成功"，而 API 返回 `code:0` + `data.status:QUEUED`；`shareMode.thread` 让所有线程重放同一批用户；bat 和 jmx 指向旧路径 `D:\couponkill\...`。`local-loadtest.ps1` 是它的无 JMeter 替代，两者都打兼容路径 `POST /order/seckill`。
- 建议方向：scripts 继续保留并沉淀为 OpenSpec 场景；`ansible/`、`linux-shell-sh/`、`test-python/` 归档出主仓；jmeter 要么按当前 API 重做，要么删除。

## 1. 范围与技术栈

### 1.0 范围与约定

- 仓库：`d:\job-project\couponkill-cloud-native`，基线 HEAD `3969213`（main）。下文路径均相对仓库根。
- 范围：`ansible/`（含 `ansible/ansible/**`）、`linux-shell-sh/`、`scripts/`、`jmeter/`、`test-python/`。
- 已执行的校验：10 个 `scripts/*.ps1` 用 `[System.Management.Automation.Language.Parser]::ParseFile` 解析，全部 0 语法错误。本机没有 `ansible-playbook` / `ansible-lint` / `ansible`（`Get-Command` 均未找到），Ansible 未做语法检查。没有执行任何脚本，也没有启动任何服务。
- 标记约定：「已核实」= 读代码或命令输出直接确认；「推断」= 依据代码加工具的文档化行为推导，未实际运行。敏感值一律不复制，只写路径、键名、是否明文。
- 路径缩写（只用于引用，便于阅读）：

| 缩写 | 实际路径前缀 |
|---|---|
| `{ans}` | `ansible/ansible/` |
| `{user}` | `couponkill-user-service/src/main/java/com/aliyun/seckill/couponkilluserservice/` |
| `{order}` | `couponkill-order-service/src/main/java/com/aliyun/seckill/couponkillorderservice/` |
| `{coupon}` | `couponkill-coupon-service/src/main/java/com/aliyun/seckill/couponkillcouponservice/` |
| `{connector}` | `couponkill-connector-service/src/main/java/com/aliyun/seckill/couponkillconnectorservice/` |
| `{gateway}` | `couponkill-gateway/src/main/java/com/aliyun/seckill/couponkillgateway/` |
| `{common}` | `couponkill-common/src/main/java/com/aliyun/seckill/common/` |

- 只写类名时的位置：`UserController.java` 在 `{user}controller/`；`OrderController.java`、`ReservationController.java`、`NotificationController.java` 在 `{order}controller/`；`AsyncSeckillEnterService.java` 在 `{order}service/`；`ReservationFireJob.java` 在 `{order}job/`；`enter_seckill.lua` 在 `couponkill-order-service/src/main/resources/lua/`；`CouponController.java` 在 `{coupon}controller/`；`ConnectorController.java` 在 `{connector}controller/`，`ConnectorAdminInterceptor.java` 在 `{connector}config/`；`JwtAuthGlobalFilter.java`、`InternalApiBlockFilter.java` 在 `{gateway}security/`。Ansible 文件只写文件名时都在 `{ans}` 下的对应子目录。

### 1.1 技术栈

本分片没有 `pom.xml` / `go.mod` / `package.json`，版本证据来自各自的清单文件。

| 子目录 | 语言 / 工具 | 关键依赖与版本（证据） |
|---|---|---|
| `ansible/ansible` | Ansible YAML + Jinja2；Python 动态清单 | `ansible>=2.15`、`ansible-lint`、`footmark`（`ansible/ansible/.github/workflows/ci.yml:22`、`ansible/ansible/ansible.sh:11`）；Python 3.11（`ci.yml:17`）；collections `alibaba.alicloud` / `community.general` / `ansible.posix` / `community.docker`，均未锁版本（`ansible/ansible/collections/requirements.yml:3-6`）；node_exporter 1.8.1（`ansible/ansible/group_vars/all.yml:31`）；目标机 JDK 包 `openjdk-25-jre-headless` / `java-25-openjdk`（`ansible/ansible/roles/deploy_service/tasks/precheck.yml:48`） |
| `scripts/` | PowerShell 5.1+ / pwsh 7，只能在 Windows 跑（用到 `Get-NetTCPConnection`、`curl.exe`、`mvn.cmd`、`cmd.exe`）；另有 1 个 Python 3 脚本 | JDK 25（`JAVA_HOME` 缺省为 `C:\Program Files\Java\latest\jdk-25`，如 `scripts/local-http-smoke.ps1:9-11`）；Maven；Docker CLI，依赖 `docker-compose.migration.yml` 的 postgres:16 / redis:7 / apache/kafka:3.8.0 / nacos/nacos-server:v3.1.1（`docker-compose.migration.yml:13,33,40,85`）；Vite `^7.1.7`（`frontend/couponkill-frontend/package.json:36`，只有 bell-ui 用到） |
| `jmeter/` | JMeter 测试计划 + Windows bat | jmx 文件格式标注 `jmeter="5.4.1"`（`jmeter/test_plan.jmx:2`）；实际运行用 JMeter 5.6.3 / Java 21.0.8（`jmeter/run_test.bat:2`、`jmeter/jmeter.log:6-7`）；采样器实现 HttpClient4（`jmeter/test_plan.jmx:92`） |
| `linux-shell-sh/` | Bash | kubectl、iostat、curl 等系统工具，无版本约束 |
| `test-python/` | Python 3 | 学习练习（requests 等），未深读 |

## 2. 结构地图

### 2.1 目录 -> 职责

| 路径 | 职责 | 状态 |
|---|---|---|
| `ansible/README.md` | DEPRECATED 横幅：禁止作为新部署入口，指向 Helm 真源（`ansible/README.md:1-8`） | 2026-07-18 加 |
| `ansible/ansible/` | 从一个独立 Ansible 仓库整体导入（`307e34a`，66 个文件、2004 行，提交信息"新学习得ansible工具"） | 冻结 |
| `ansible/ansible/ansible.cfg` | `inventory = inventories/alicloud`、`host_key_checking = False`、jsonfile 事实缓存；没有 `roles_path`（`ansible.cfg:1-17`） | |
| `ansible/ansible/ansible.sh` | 命令备忘录（无 shebang，不是可执行脚本），含占位 AK/SK export 和 prod 部署命令样例 | |
| `ansible/ansible/.ansible-lint.yml` | production profile；`mock_modules` 里放了 `community.general.ali_slb_backend`（`:15-17`） | |
| `ansible/ansible/.github/workflows/ci.yml` | 嵌套的"Ansible CI"：syntax-check 3 个 playbook + ansible-lint + 无凭据清单冒烟（`:1-36`） | 永不触发 |
| `ansible/ansible/inventories/alicloud/alicloud.py` | 阿里云官方旧版 footmark ECS 动态清单脚本（GPLv3 头，`:1-19`） | |
| `ansible/ansible/inventories/alicloud/alicloud.ini` | 清单配置：tag 过滤、分组开关、缓存；尾部混入 bash export（`:50-57`） | |
| `ansible/ansible/inventories/alicloud/README.md` | 空文件（0 字节） | |
| `ansible/ansible/group_vars/` | `all.yml`、`tag_app=yourapp.yml`、`tag_env=prod.yml`、`tag_role=web.yml` 有内容；`tag_env=staging.yml`、`tag_role=api.yml`、`tag_role=db.yml` 为 0 字节 | 全是模板占位 |
| `ansible/ansible/host_vars/`、`templates/`、`vault/` | 只有 `.gitkeep`；`vault/yourapp.yml` 1 行 | |
| `ansible/ansible/files/keys/devops.pub` | 唯一的密钥文件：ssh-rsa 公钥 | |
| `ansible/ansible/playbooks/` | `bootstrap` / `provision_ecs` / `deploy` / `rollback` / `observability` | |
| `ansible/ansible/roles/` | `arms_agent` `common` `deploy_service` `docker` `nginx` `node_exporter` `prometheus_rules` `sls_logtail` | |
| `ansible/ansible/*.json` | 4 份 RAM 策略文档（`RAM-role.json`、`AnsibleOssReadArtifacts.json`、`AnsibleSlbBackendManage.json`、`AnsibleSlsWrite.json`），没有代码引用 | |
| `linux-shell-sh/` | `shell.sh`（429 行，系统与 K8s 排查菜单）、`shell1.sh`（686 行，Pod 部署/排障/清理）、`shell2.sh`（1361 行，root 一键装开发环境） | 冻结、无引用 |
| `scripts/` | 本地联调工具链：10 个 `.ps1` + 1 个未跟踪的 `.py` | 活跃 |
| `jmeter/` | `test_plan.jmx`、`jmeter.properties`、`performance.properties`、`run_test.bat`、`user_tokens.csv`、`jmeter.log` | 半废弃 |
| `test-python/` | 65 个文件：basic-code、data-code、err-code、file-code、gc-code、math-code、mathtools、my-code、net-code、project、python-object、thread-code、mytest.py | 无关 |

### 2.2 Ansible playbook 与 role

| playbook | 流程（已核实，读代码） |
|---|---|
| `playbooks/bootstrap.yml:1-11` | `hosts: "tag_env=prod"`，become；依次跑 `common` -> `docker` -> `node_exporter` -> `sls_logtail` -> `arms_agent` |
| `playbooks/provision_ecs.yml:1-18` | 名为 provision，实际只在 localhost 调 `community.general.ali_instance_info` 列实例并打印数量，不创建任何资源 |
| `playbooks/deploy.yml:1-112` | `hosts: "tag_env=prod:&tag_app=yourapp"`。pre_tasks（`:10-39`）在 localhost 按 `canary_batch` 比例随机选主机，`add_host` 建 `canary` / `non_canary` 组；tasks 按 `deploy_strategy` 分三支：blue_green（`:43-72`，算 next_color -> include `deploy_service` -> 渲染 nginx upstream -> 可选 SLB 摘挂）、canary（`:74-99`，canary 批次 -> `pause promote_wait` -> 其余滚动）、rolling（`:101-106`）；handler `Reload nginx`（`:108-112`） |
| `playbooks/rollback.yml:1-48` | `ls -1dt releases/*` 按修改时间排序，取第 `rollback_steps` 个，`current` 软链指过去，jar/binary 重启 systemd、container 重新 `compose up`，最后跑 healthcheck |
| `playbooks/observability.yml:1-7` | `hosts: prometheus`，跑 `prometheus_rules` |

| role | 职责（设计意图） | 使用方 | 现状（证据见第 6 节） |
|---|---|---|---|
| `common` | 时区、基础包、建 `devops` 用户并下发公钥、sshd 加固、chrony、sysctl | bootstrap | tasks 文件末尾内嵌 `handlers:`（`roles/common/tasks/main.yml:60`）；`sshd_config.j2` 不存在（`:34-35`，`roles/common/templates/` 目录不存在）；`disable_root_ssh` / `password_auth` / `fail2ban_enabled` / `sudo_nopasswd`（`group_vars/all.yml:8,12-14`）没有任何任务消费 |
| `docker` | 装 Docker CE + buildx + compose 插件 | bootstrap | `args: { warn: false }`（`roles/docker/tasks/main.yml:15`） |
| `node_exporter` | 从 GitHub 下载 1.8.1、建 systemd 单元 | bootstrap | 内嵌 `handlers:`（`roles/node_exporter/tasks/main.yml:39`）；下载无校验和（`:8-13`） |
| `sls_logtail` | 装 iLogtail、渲染采集配置 | bootstrap | 模板名 `ilogtail-{{ app_name }}.json.j2`（`roles/sls_logtail/tasks/main.yml:35-36`）与实际文件 `templates/templates/ilogtail.json.j2` 对不上；url 安装分支缺 `fi`（`:19-25`） |
| `arms_agent` | 下载 ARMS Java 探针，经 systemd drop-in 或 `container_env` 注入 `JAVA_TOOL_OPTIONS` | bootstrap | `arms_enabled`（`defaults/main.yml:1`）从不判断；AK/SK 渲染到 0644 文件（`tasks/main.yml:15-19`） |
| `deploy_service` | precheck -> fetch_artifact -> render_configs -> `deploy_{container,jar,binary}` -> healthcheck -> finalize（`roles/deploy_service/tasks/main.yml:1-13`）；另有 `traffic_drain` / `traffic_attach` | deploy、rollback | compose 模板缩进错、健康检查 `/healthz`、多处 `warn` |
| `nginx` | 装 nginx、渲染站点配置 | 没有任何 playbook 使用（死代码） | 内嵌 `handlers:`（`roles/nginx/tasks/main.yml:35`）；`site.conf.j2:1-7` 把 `worker_processes` / `events` / `http` 顶层指令写进 `conf.d` |
| `prometheus_rules` | 渲染节点/应用告警规则与 Alertmanager 配置，`kill -HUP` 重载 | observability | 模板里的 `{{ $labels.x }}` 未转义 |

### 2.3 scripts/

| 脚本 | 用途 | 前置依赖 | 引入提交 |
|---|---|---|---|
| `import-nacos-local.ps1` | 把仓库 `nacos/` 配置灌进本地 Nacos 3.x，并把容器主机名改写为 localhost | Nacos 在 `127.0.0.1:8848`（compose 的 `couponkill-nacos`）、`curl.exe` | 6b3e9b5 |
| `local-http-smoke.ps1` | 起 user / gateway / order / coupon（固定端口），做 JWT 登录 + 订单双路径 + Long ID 字符串契约冒烟 | compose 起好中间件、已导入 Nacos、JDK 25、mvn | 747677d |
| `local-notification-smoke.ps1` | 起 user / gateway / order（空闲端口），种 1 条站内通知，校验未读数和列表 | 同上 + `docker exec couponkill-postgres psql` | 02c33c4 |
| `local-price-compare-smoke.ps1` | 起 gateway + connector，补建 `coupon_price_map`，用 admin token 写绑定和手工价，再匿名比价 | 同上 + `connector_db` | 30deb3b |
| `local-reservation-firedue-e2e.ps1` | 预约帮抢 fireDue 全链路 E2E（建窗 -> 预约 -> 开窗预热 -> 提前触发 -> 轮询 -> 查通知） | 同上 + Kafka、Redis 容器 | 0e10e22 |
| `local-bell-ui-up.ps1` / `local-bell-ui-down.ps1` | 起 user / gateway / order + Vite，让人在浏览器里手测通知"铃铛"（`frontend/couponkill-frontend/src/components/Header.tsx:15,48-57`）；up 写状态文件后退出、进程常驻，down 按状态文件和命令行清理 | 同上 + Node 与前端 `node_modules` | 0e10e22 |
| `local-loadtest.ps1` | 无 JMeter 的小规模秒杀并发验证：注册/登录 N 个用户，并发打 `/order/seckill`，看 Redis 库存差 | 服务已在运行（网关缺省 `http://127.0.0.1:8088`）+ `couponkill-redis` 容器 | 6b3e9b5 |
| `jdk25-local-env.ps1` | dot-source 后设置 `JAVA_TOOL_OPTIONS`（ZGC、ZGenerational、紧凑对象头、MaxRAMPercentage=75、`--enable-preview`） | JDK 25 | 6b3e9b5 |
| `jdk25-aot-train.ps1` | JDK Leyden AOT 训练：以 `-XX:AOTMode=record` 跑 order-service jar，产出 `order.aotconf`；没有 create AOTCache 这一步 | 已构建的 order-service jar；JDK 路径硬编码为 `D:\dev-lanuage\java\jdk-25`（`:11`） | 6b3e9b5 |
| `_tmp_migrate_user_errorcodes.py` | 一次性 codemod：把 `UserServiceImpl.java` 的 `ResultCode` 换成 `ErrorCodes` | Python 3，必须在仓库根运行（相对路径，`:4-7`） | 未跟踪（`git status` 显示 `??`） |

### 2.4 jmeter/

| 文件 | 内容 |
|---|---|
| `test_plan.jmx` | 1 个线程组：50 线程、ramp 5s、调度 30s、循环 -1、出错继续（`:25-33`）；CSV Data Set 读 `user_tokens.csv`（`:40-48`）；`POST http://localhost:8088/order/seckill`，参数 `userId`、`couponId=1001`，头 `Authorization: Bearer ${token}`（`:11-16,55-93`）；响应断言要包含"秒杀成功"和"200"（`:98-104`）；聚合报告写绝对路径（`:183`） |
| `user_tokens.csv` | 只有表头 `userId,token` 和 1 个空行，没有数据行 |
| `run_test.bat` | 非 GUI 运行：`jmeter -n -t ... -q performance.properties -l ... -j ...`，路径全是 `D:\couponkill\couponkill-cloud-native\...`（`:2-14`） |
| `performance.properties` | 通过 `-q` 追加的调参（JVM、HttpClient、连接池、报表阈值） |
| `jmeter.properties` | 另一份调参，没有被 bat 引用 |
| `jmeter.log` | 2025-09-01 的一次运行日志（已提交） |

### 2.5 linux-shell-sh/ 与 test-python/

- `linux-shell-sh/`：与 couponkill 无关的通用运维学习脚本，全文没有 couponkill / seckill 字样；除本目录外全仓没有引用（已核实，rg 检索）。`shell.sh` 是交互式排查菜单（`:385-399`）；`shell1.sh` 是 Pod 部署、故障分析、连通性测试、非运行 Pod 清理（`:44,118,221,370,518`）；`shell2.sh` 要求 root，一键安装 Python / Java / Node / Go / Docker / kubectl / helm / minikube / terraform / ansible / istioctl / kind / 各云 CLI 等（`:5-9,131-640`）。
- `test-python/`：结论一句话，与本项目无关的 Python 学习练习（rg 检索 coupon、seckill、couponkill、8088、nacos、jwt、kafka、postgres、redis 均无命中，只有对示例 URL 的 `requests.get`）。目录清单：`basic-code/`、`data-code/`（含 `sales_report.xlsx`、`product_sales.csv`、`daily_sales.json`、`product_report.html`）、`err-code/`、`file-code/`、`gc-code/`、`math-code/`（merge_sort、quick_sort）、`mathtools/`（basic、stats 子包）、`my-code/`、`net-code/`（socket 客户端/服务端、requests）、`project/`（student、log、while 等练习）、`python-object/`、`thread-code/`、`mytest.py`。

## 3. 核心流程

### 3.1 本地联调总链路

```
 dev box (Windows, pwsh)
   |
   | (1) docker compose -f docker-compose.migration.yml up -d
   v
 +--------------------------------------------------------------------+
 | couponkill-postgres  host 5433 -> 5432   (initdb: 01/02/03 *.sql)  |
 | couponkill-redis     host 6379                                     |
 | couponkill-kafka     host 9092   (in-network kafka:29092)          |
 | couponkill-nacos     host 8848 / 9848   (auth disabled)            |
 +--------------------------------------------------------------------+
   |
   | (2) scripts/import-nacos-local.ps1
   |     POST /nacos/v1/console/namespaces   -> ns 120, ns 998
   |     POST /nacos/v1/cs/configs  x N      -> Localize(): postgres:5432 => 127.0.0.1:5433 ...
   v
 [ Nacos ns 120: DEFAULT_GROUP/*.yaml + couponkill-<svc>.yaml + SENTINEL_GROUP ]
 [ Nacos ns 998: shard/DEFAULT_GROUP/*.yaml                                   ]
   |
   | (3) any smoke script
   |     mvn -pl couponkill-common -am install
   |     Start-Process mvn.cmd -f <module>/pom.xml spring-boot:run   (env SERVER_PORT, NACOS_*)
   |     Wait-Port --> Assert-PortOwnedBy (listener cmdline must match the module)
   v
 [ gateway ] --JWT--> [ user | order | coupon | connector ]
   |
   | (4) curl.exe assertions: code == 0, Long IDs as JSON strings, business states
   v
 [ finally: Stop-SmokeProcs  (kill by PID, then by java.exe cmdline regex) ]
```

每一跳的文件：
- (1) `docker-compose.migration.yml:12-99`：容器名 `couponkill-postgres` / `couponkill-redis` / `couponkill-kafka` / `couponkill-nacos`（`:14,34,41,86`）；PG 宿主机端口 5433（`:20`）；首次初始化只挂载 `init-postgres.sql`、`02-seed-demo.sql`、`03-init-connector.sql`（`:22-24`）；Nacos 关闭鉴权（`:92`）。
- (2) `scripts/import-nacos-local.ps1:10-84`，细节见 4.2。
- (3) `scripts/local-http-smoke.ps1:56-68`（启动，注释要求必须 `-f` 子模块 POM，不能 `-am spring-boot:run`）、`:37-54`（端口归属校验）、`:85-87`（先装 common）。
- (4) `scripts/local-http-smoke.ps1:129-225`；清理 `:70-81`、`:227-230`。

### 3.2 预约 fireDue E2E（`scripts/local-reservation-firedue-e2e.ps1`）

```
 e2e script          gateway        order-svc               coupon-svc      PG order_db_0    Redis
    |  login ---------->|--> user-svc  |                        |               |              |
    |  GET resv/mine -->|------------->|                        |               |              |
    |  DEL resv/{id} -->|------------->|  (PENDING only)        |               |              |
    |  psql: UPDATE seckill_reservation -> CANCELLED ---------------------------->|              |
    |  POST coupon/1001/seckill-window (future) ----- direct --->|               |              |
    |  POST resv {"couponId":"1001"} -->|------------>|  expect PENDING          |              |
    |  POST coupon/1001/seckill-window (open) ------- direct --->|               |              |
    |  redis-cli DEL seckill:cooldown:10000:1001 seckill:deduct:10000:1001 ------------------------>|
    |  POST coupon/preheat-stock/1001 --------------- direct --->|-- SET coupon:stock:1001 ----->|
    |  psql: UPDATE trigger_at = NOW() - 1 minute ------------------------------->|              |
    |                                  | ReservationFireJob (@Scheduled)           |              |
    |                                  |   fireDueReservations -> Lua enter -> Kafka              |
    |                                  |   syncQueuedResults                       |              |
    |  poll GET resv/{id} (120s, +60s if QUEUED) ---->|                            |              |
    |  GET notifications/unread-count, /mine -------->|                            |              |
    v  PASS if status in {SUCCESS, QUEUED}
```

每一跳的文件：
- 登录 `:174-179` -> `couponkill-user-service/src/main/java/com/aliyun/seckill/couponkilluserservice/controller/UserController.java:28`。
- 清理残留预约 `:184-200`：API 取消 -> `ReservationController.java:43`（只允许取消 PENDING，`:42`）；SQL 兜底把 PENDING / FIRING / QUEUED 改成 CANCELLED，理由是唯一活跃约束（`charts/couponkill/scripts/04-seckill-reservation.sql:66-68`）。
- 改活动窗口 `:125-136,202-203,226-227`：直连 coupon-service，注释说明网关会拦（`:181`），对应 `CouponController.java:98-110`、网关 `JwtAuthGlobalFilter.java:143-153`。
- 创建预约 `:205-224` -> `ReservationController.java:31-40`，要求返回 `PENDING`。
- 清用户维度键 `:228`，键名定义在 `AsyncSeckillEnterService.java:65,69` 与 `enter_seckill.lua:3-4`。
- 预热 `:230-234` -> `CouponController.java:219`（网关对 `/preheat-stock` 有拦截，`InternalApiBlockFilter.java:39`）。
- 提前触发 `:241-249`（直接改 `trigger_at`）；调度器 `couponkill-order-service/src/main/java/com/aliyun/seckill/couponkillorderservice/job/ReservationFireJob.java:22-26`。
- 轮询与判定 `:251-318`（第 6 节第 3 行说明断言偏弱）。

### 3.3 Ansible 设计流程与断点

```
 ansible-playbook -i inventories/alicloud playbooks/deploy.yml -l "tag_env=prod:&tag_app=yourapp"
   |
   v
 [ inventory: alicloud.py (footmark) ]
   |   x(1) import ansible.module_utils.alicloud_ecs  -> not shipped by ansible-core >= 2.10
   |   x(2) footmark missing -> "footmark.ecs = None" raises AttributeError
   |   x(3) regions = all needs ecs:DescribeRegions, not granted by RAM-role.json
   v
 [ groups: cn-hangzhou, type_*, vpc_id_*, security_group_*, tag_env_prod, tag_app_yourapp, alicloud ]
   |   x(4) to_safe() maps '=' to '_'  -> play hosts "tag_env=prod:&tag_app=yourapp" match nothing
   |   x(5) group_vars/ sits at project root -> not loaded (not beside inventory or playbooks)
   v
 [ role deploy_service ]
   |   x(6) no roles_path and no playbooks/roles/ -> role not found
   |   precheck -> fetch_artifact -> render_configs -> deploy_<container|jar|binary>
   |   x(7) docker-compose.yml.j2 mis-indented; shell tasks pass removed arg "warn"
   v
 [ healthcheck GET http://127.0.0.1:8080/healthz ]
   |   x(8) couponkill services expose /actuator/health (Go: /health)
   v
 [ finalize: write .release, purge releases beyond keep_releases ]
```

动态清单如何按 tag 分组（已核实，`ansible/ansible/inventories/alicloud/alicloud.py:342-462` + `alicloud.ini:11-48`）：
- 过滤：只拉 `tag:env=prod` 且 `tag:managed=ansible`、状态为 running 的实例（`alicloud.ini:17-18,44`）；`regions = all` 时先经 cn-beijing 调 `describe_regions()` 再逐地域遍历（`alicloud.py:194-207`）。
- 主机名取 ECS 标签 `name`（`alicloud.ini:22`，经 `to_safe()` 并小写）；连接地址取私网 IP（`alicloud.ini:21`），写进 `ansible_ssh_host`（`alicloud.py:462`），意味着必须在 VPC 内或经跳板机执行。
- 分组：地域、可用区、`type_<规格>`、`vpc_id_<id>`、`subnet_<vswitch>`、`security_group_<id>`（开关在 `alicloud.ini:25-36`）；每个标签生成 `tag_<k>_<v>`，值含逗号时拆成多个组（`expand_csv_tags = True`），并建嵌套组 `tags -> tag_<k> -> tag_<k>_<v>`（`alicloud.py:443-451`）；没有标签的实例进 `tag_none`（`:454-457`）；所有主机进 `alicloud`（`:459`）。
- 关键缺陷：`to_safe()`（`alicloud.py:544-549`）把 `[A-Za-z0-9_]` 以外的字符（包括 `=` 和 `-`，因为 `replace_dash_in_groups = True`，`:78`）都替换成 `_`，所以清单里根本不存在 `tag_env=prod` 这种组名，而 playbook（`bootstrap.yml:3`、`deploy.yml:3,8`、`rollback.yml:3`）和 group_vars 文件名都用 `=` 形式。

断点证据：(1) `alicloud.py:28`（推断）；(2) `alicloud.py:42-46`（已核实，Python 语义上对 None 赋属性必抛异常）；(3) `alicloud.ini:13` vs `ansible/ansible/RAM-role.json:6-18`（已核实策略里没有 `ecs:DescribeRegions`，运行结果为推断）；(4) 已核实；(5)(6) `ansible.cfg` 没有 `roles_path`，`playbooks/roles`、`playbooks/group_vars`、`inventories/alicloud/group_vars` 都不存在（已核实），按 Ansible 文档化的查找规则推断不加载；(7) `roles/deploy_service/templates/docker-compose.yml.j2:2-8`，`warn` 见第 6 节；(8) `group_vars/tag_app=yourapp.yml:36-42` vs `charts/couponkill/templates/deploy-user.yaml:52,59`。

### 3.4 HTTP 冒烟（`scripts/local-http-smoke.ps1`）

脚本不设 `SERVER_PORT`。它假定 Nacos 里的端口已经是 user 8081、gateway 8088、order 8082、coupon 8080（`Start-Module` 调用在 `:95-98`），然后 `Wait-Port` 并核对监听进程命令行（`:107-118`）。登录、双路径订单、券列表、可选的常驻下单都打 `http://127.0.0.1:8088`（`:131,159-169,213`）。

清理分两步：先杀掉 `Start-Process` 拿到的 `mvn.cmd` PID（`:70-76`），再按 `java.exe` 命令行匹配 `couponkill-(user|gateway|order|coupon)-service`（`:78-80`）。网关模块目录是 `couponkill-gateway`，主类包名是 `couponkillgateway`，两者都不包含子串 `couponkill-gateway-service`。fork 出来的 JVM 若只带模块路径或主类名，这条正则匹配不到【推断：Spring Boot 插件的 fork 命令行形态未在本机抓取】。端口归属校验用的正则是 `couponkillgateway|couponkill-gateway`（`:103`），和清理正则不是同一条。

同目录里 `local-notification-smoke.ps1`、`local-price-compare-smoke.ps1`、`local-bell-ui-up.ps1`、`local-reservation-firedue-e2e.ps1` 都会在启动前写 `$env:SERVER_PORT`（例如 price-compare `:69`）。http-smoke 是例外。

### 3.5 通知冒烟、比价冒烟、铃铛

- `local-notification-smoke.ps1`：起 user / gateway / order，用 `docker exec` 往 `user_notification` 插一行，再经网关看未读数和列表。成功条件把业务 `code` 的 `0` 和 `200` 都当成通过（`:150,160`）。`ApiResponse.success` 的 code 是 0（`couponkill-common/.../api/ApiResponse.java:25-26`）。`200` 这条是放宽，不是当前成功体。
- `local-price-compare-smoke.ps1`：起 gateway + connector，补建价表，用 admin token 写绑定和手工价，再匿名调用比价（`:172-199`）。匿名比价与网关白名单、connector 拦截器放行一致（07a 第 3 节）。
- `local-bell-ui-up.ps1` / `local-bell-ui-down.ps1`：起后端和 Vite，把 PID 写到状态文件后退出。down 按状态文件和命令行清理。前端铃铛位置见 04，这里不重复组件。

### 3.6 无 JMeter 压测（`scripts/local-loadtest.ps1`）

```
 SET coupon:stock:{couponId} = 1600          (:12)  不删 seckill:deduct / user:received
 循环注册+登录 N 个用户（口令在 query）       (:16-21)
 每个用户一个 Job：
   POST {gateway}/order/seckill?couponId
   Authorization: Bearer {token}
   另外带上 X-User-Id（经网关时由 JWT 过滤器覆写，见 01 第 3.2 节）
 统计 body 里是否出现 "status":"QUEUED"
 3 秒后 GET 同一库存键，用 1600 减当前值当「消耗」  (:56-62)
```

重复跑不会清用户维度的预扣键和领取标记。第二次同一批用户名会走注册失败或 Lua `-3` / 唯一约束，`queued` 计数不再表示新的库存预扣【推断：键的 TTL 和唯一约束来自 02 第 3.1 节，脚本本身没有 DEL】。库存差也不等于 DB 扣减，因为 Redis 与 DB 本来就不是同一本账（02 第 6 节第 3 行）。

### 3.7 JMeter 计划与 JDK 辅助脚本

`jmeter/test_plan.jmx` 的采样器是 `POST http://localhost:8088/order/seckill`（第 2.4 节）。CSV 只有表头。断言要求响应包含「秒杀成功」（`test_plan.jmx:99`）和「200」（`:98-104`）。当前热路径成功体是 `code:0` 且 `data.status` 为 `QUEUED`（02 第 4 节），没有「秒杀成功」这四个字。`shareMode.thread`（`:48`）的注释写「每个线程使用独立数据」；JMeter 文档里 `shareMode.thread` 是线程内各自走自己的游标，CSV 若只有一行数据，每个线程仍只会反复读到空行【推断：JMeter 5.4 CSV Data Set 的 shareMode 语义，未运行】。`run_test.bat` 的路径是 `D:\couponkill\...`（第 2.4 节）。

`jdk25-local-env.ps1:3` 把 `JAVA_TOOL_OPTIONS` 设成带 `-XX:+ZGenerational` 的一串。该 flag 在 JDK 24 起 obsolete、JDK 25 只警告，依据与 05 报告 R45 相同。`jdk25-aot-train.ps1` 只用 `-XX:AOTMode=record` 写出 `order.aotconf`（`:16-21`），没有 `AOTMode=create`，也没有任何启动脚本读取这个文件。JDK 路径写死为 `D:\dev-lanuage\java\jdk-25\bin\java.exe`（`:11`）。

`_tmp_migrate_user_errorcodes.py` 未被 git 跟踪（第 2.3 节）。它按相对路径改 `UserServiceImpl.java`。当前 user-service 已经使用 `ErrorCodes`（01 第 4 节），这个脚本是一次性 codemod 的残留。

## 4. 对外契约

本分片没有自己的 HTTP 服务。契约是「脚本调用了哪些后端接口」以及「Ansible 假设目标机长什么样」。

### 4.1 scripts 调用的后端接口

| 脚本 | 调用 | 后端位置（已在 Controller 对上） |
|---|---|---|
| http-smoke、loadtest、firedue、notification、bell | `POST /api/v1/user/login`，表单 `username`/`password` | `UserController.java` 登录方法；口令在 query/form 的风险见 01 |
| http-smoke | `GET /order/user/me` 与 `GET /api/v1/order/user/me` | `OrderController.java:76-77` 双前缀 |
| http-smoke | `GET /api/v1/coupon/available`；可选 `POST /api/v1/order/create?couponId=1002` | `CouponController.java:34`；`OrderController.java:51` |
| loadtest、jmx | `POST /order/seckill?couponId=` | `OrderController.java:113` |
| firedue | 预约 CRUD、直连 coupon 的 seckill-window 与 preheat | 第 3.2 节的文件表 |
| notification、bell | `GET /api/v1/order/notifications/unread-count` 与 `/mine` | `NotificationController.java:30-38` |
| price-compare | connector 绑定、价表、`GET /price-compare` | `ConnectorController.java:50-69` |
| import-nacos | `POST /nacos/v1/console/namespaces`、`POST /nacos/v1/cs/configs` | 05 报告 4.7；鉴权在 compose 里关闭 |

firedue 直连 coupon 能改时间窗和预热，是因为这两个接口在服务内部不鉴权，网关才拦截（02 第 4 节，`InternalApiBlockFilter.java:39`，`JwtAuthGlobalFilter.java:146-153`）。脚本注释写了「网关会拦」（firedue `:181`），所以改走直连。这是跨分片观察，不是脚本单独的缺陷。

### 4.2 导入脚本改写了什么

`Localize`（`import-nacos-local.ps1:25-37`）把 `postgres:5432` 改成 `127.0.0.1:5433`，把 `redis` / `redis-master`、`kafka:9092`、`kafka:29092`、`nacos:8848` 改到本机。另有一条把行首两空格的 `port: 80` 改成 `8088`（`:37`）。当前 `nacos/` 下没有匹配 `^  port: 80$` 的行【已核实，检索】，所以这条今天不会改任何已跟踪配置。它不改 namespace 字符串 `couponkill`（07b 第 1.3 节第 2 条）。

发布范围：`nacos/DEFAULT_GROUP/*.yaml` 进 tenant 120，并额外复制 `couponkill-<svc>.yaml`；`SENTINEL_GROUP` 一份；`shard/DEFAULT_GROUP` 进 tenant 998（`:63-83`，细节与 05 第 4.3 节同一张表）。`service-collaboration` 无扩展名，被 `-Filter *.yaml` 丢掉。

### 4.3 Ansible 假设的目标

| 假设 | 文件 | 与 couponkill 的关系 |
|---|---|---|
| 主机组名 `tag_env=prod`、`tag_app=yourapp` | `playbooks/deploy.yml:3` | 动态清单生成的组名是 `tag_env_prod`（第 3.3 节） |
| 应用端口 8080，健康检查 `GET /healthz` | `group_vars/tag_app=yourapp.yml:36-42` | Java 探针路径在 Chart 里是 `/actuator/health`，而且 pom 没有 actuator（05 R30） |
| 制品是 `yourapp` 的 jar / compose | `roles/deploy_service/` | 没有 couponkill 的服务名、镜像名、Nacos |
| 事实缓存、关闭 host key 检查 | `ansible.cfg:1-17` | 没有 `roles_path` |
| RAM 角色能列 ECS | `RAM-role.json` | 策略动作列表里没有 `ecs:DescribeRegions`（第 3.3 节断点 3） |

`linux-shell-sh/` 与 `test-python/` 不调用本仓库的接口（第 2.5 节）。

## 5. 依赖关系

```
开发机 Windows
  -> docker compose -f docker-compose.migration.yml     （05 的本地链路）
  -> scripts/import-nacos-local.ps1 -> 本机 Nacos 8848
  -> scripts/local-*.ps1
        -> mvn.cmd -f <module>/pom.xml spring-boot:run
        -> curl.exe 打网关或直连 coupon
        -> docker exec couponkill-postgres / couponkill-redis
  -> frontend vite（仅 bell-ui）

ansible/ansible  （DEPRECATED，README 指向 Helm）
  -> 阿里云 ECS API（footmark + alicloud.py）
  -> 目标机 systemd / docker compose / nginx
  -> 不读取 charts/、nacos/、Jenkinsfile

jmeter/  -> 假定本机 8088 上已有网关，CSV 提供 JWT
linux-shell-sh/、test-python/  -> 无仓库内引用
```

和其它分片的接口：

- 05：compose 服务名、`import-nacos-local.ps1` 是本地配置真源；本分片把脚本步骤写完，不重复 Helm 断裂点。
- 01 / 02：脚本打到的登录、秒杀、预约、比价契约。firedue 直连写接口，用来说明服务自身没有鉴权。
- 04：bell-ui 只负责把 Vite 拉起来，页面行为在前端分片。
- 07b：导入后的 dataId、namespace、端口。`Localize` 不改 `spring.cloud.nacos.namespace: couponkill`。

`ansible/ansible/.github/workflows/ci.yml` 放在子目录。GitHub Actions 只识别仓库根的 `.github/workflows/`【推断：GitHub 文档规定的工作流路径】。根上的 CI 是 `.github/workflows/ci.yml`，不包含 Ansible。嵌套这份 workflow 不会在 push 时运行。

## 6. 问题与风险

第 0 节的五条在这里按文件落点写清。活跃脚本只打本机，没有单独标成「高」。

| # | 级别 | 类型 | 结论 | 证据 |
|---|---|---|---|---|
| 1 | 中 | 正确性 | Ansible 按现状跑不通，也不是 couponkill 的部署入口。`to_safe()` 把 `=` 换成 `_`，play 的主机模式却写 `tag_env=prod`。`ansible.cfg` 无 `roles_path`，`group_vars/` 不在清单或 playbook 旁边。3 个 tasks 文件在 tasks 里内嵌 `handlers:`（`roles/common/tasks/main.yml:60`、`roles/node_exporter/tasks/main.yml:39`、`roles/nginx/tasks/main.yml:35`）。10 处 `args.warn`（`roles/docker/tasks/main.yml:15` 以及 sls、prometheus_rules、nginx、deploy_service 的四份 tasks、`playbooks/rollback.yml:43`）。健康检查是 `/healthz`，制品名是 `yourapp`。嵌套 CI 不在仓库根 `.github/workflows` | 第 3.3 节；`ansible/README.md:1-8` 已标 DEPRECATED |
| 2 | 低 | 安全 | `vault/yourapp.yml` 没有 ansible-vault 头，内容是一行 `community.hashi_vault` lookup，不是口令正文，且全仓没有引用。`files/keys/devops.pub` 是公钥，结构不构成可用私钥【已核实：只有这一个密钥文件】。`AnsibleSlbBackendManage.json` 的写操作 `Resource` 是 `*`，策略里没有 `Action:*` | `{ans}vault/yourapp.yml`；`{ans}files/keys/devops.pub`；`{ans}AnsibleSlbBackendManage.json` |
| 3 | 低 | 测试 | fireDue E2E 把 `SUCCESS` 和 `QUEUED` 都当 PASS，通知缺失只打 WARN（`local-reservation-firedue-e2e.ps1:267-314`）。直连 coupon 写时间窗和预热能够成功，说明这两条写接口的鉴权在网关而不是服务里 | 第 3.2 节；02 第 4 节 |
| 4 | 低 | 脚本 | http-smoke 的 java 清理正则对不上网关的目录名和主类包名（`:79` 对 `:103`）。loadtest 不清理 `seckill:deduct:*` 与 `user:received:*`（`local-loadtest.ps1:12` 只有 SET）。AOT 脚本只 record、不 create，JDK 路径硬编码（`jdk25-aot-train.ps1:11,19-20`）。`ZGenerational` 见 `jdk25-local-env.ps1:3` 与 `jdk25-aot-train.ps1:17`。`_tmp_migrate_user_errorcodes.py` 未跟踪 | 第 3.4、3.6、3.7 节 |
| 5 | 中 | 测试 | JMeter 计划对不上当前 API：CSV 无数据行；断言找「秒杀成功」；bat 指向 `D:\couponkill\...`；`shareMode.thread` 在只有表头时不会给每个线程不同的用户。`local-loadtest.ps1` 是替代，两者都打旧前缀 `/order/seckill`。该前缀今天仍然由 `OrderController` 提供，所以路径本身还在，断言和数据不在 | `jmeter/test_plan.jmx:40-48,92-104`；`jmeter/user_tokens.csv`；`jmeter/run_test.bat:2-14` |
| 6 | 低 | 测试 | notification 冒烟把 `code==200` 也算成功。当前成功体是 `code==0` | `local-notification-smoke.ps1:150,160`；`ApiResponse.java:25-26` |
| 7 | 低 | 脚本 | http-smoke 不导出 `SERVER_PORT`，完全依赖 Nacos 端口。其它冒烟脚本会设置。Nacos 没导入时五个进程的 Spring 默认端口都是 8080，等待会失败或互相抢端口【推断：Spring Boot 默认 8080，与 07b 第 1.2 节相同】 | `local-http-smoke.ps1:56-68,95-98` 对比 `local-price-compare-smoke.ps1:69` |
| 8 | 低 | 无关资产 | `linux-shell-sh/` 三份交互脚本、`test-python/` 65 个练习文件与 couponkill 无引用。`shell2.sh` 要求 root 并安装一长串工具链，不应出现在部署文档里 | 第 2.5 节 |

没有把 ansible 标成「高」：目录已废弃，而且第 1 行的断点会在真正改主机之前失败（清单组名对不上、role 找不到）。这是 fail-closed，不是一套还能部署生产的错误剧本。

JWT 曾经进过 `user_tokens.csv`。`6b3e9b5` 删掉了数据行，历史提交里仍能看到约 3076 条【已核实：当前文件只有表头；条数来自该提交的删除规模，本分片没有把 token 再打印出来】。那些 token 若对应仍有效的 `jwt.secret`，在过期前仍是凭证。过期时间见 01（Nacos 写 86400000 ms）。是否已经轮换密钥，本次没有查密钥管理系统。

## 7. 最近活跃度

`git log` 按目录计数（一条提交可以跨目录，所以下面不是相加关系）：

| 路径 | 提交数 | 最后一次 |
|---|---|---|
| `scripts/` | 10 | 2026-07-18 `47cd445`（时间窗全分片；脚本侧跟着改） |
| `ansible/` | 3 | 2026-07-18 `4bcf536`（文档横幅，不是剧本功能） |
| `jmeter/` | 9 | 2026-07-12 `6b3e9b5`（删掉 CSV 里的 JWT，计划本身没按新 API 改） |
| `linux-shell-sh/` | 1 | 2025-09-20 `89d1ca7` |
| `test-python/` | 3 | 2025-10-03 `ecf0e2a` |

`scripts/` 在 2026-07-12 至 07-18 的提交就是本地联调链：`6b3e9b5` 导入与压测，`747677d` HTTP 冒烟，`5ec6740` / `f78b350` 端口归属，`30deb3b` 比价，`02c33c4` 通知，`0e10e22` fireDue 与铃铛。这和 05 第 7 节的判断一致：最近被跟着改的是 compose + 脚本，不是 Ansible。

## 8. 未验证 / 不确定项

- 10 个 `scripts/*.ps1` 只做了 PowerShell 语法解析（第 1.0 节），没有执行。端口清理是否真的留下 8088 上的 JVM，没有在本机起过 spring-boot:run。
- 本机没有 `ansible-playbook` / `ansible-lint`。tasks 里内嵌 `handlers:`、`warn` 在 2.14 被移除、`roles_path` 的查找顺序，都是按 Ansible 文档推断的失败点，没有跑 syntax-check。
- `alicloud.py` 在 ansible-core ≥ 2.10 缺少 `ansible.module_utils.alicloud_ecs`，以及 `footmark` 未安装时对 `None` 赋属性会抛异常，是读 import 和 Python 语义得到的，没有装那个解释器。
- JMeter `shareMode.thread` 在空 CSV 上的具体报错文案没有跑 JMeter 5.6.3 复现。断言字符串对不上当前 JSON，是把 jmx 和 02 的成功体对过文本。
- `6b3e9b5` 删除的 JWT 是否仍能通过今天仓库里的占位密钥验签，没有把历史 blob 签出来验证。
- `Localize` 的 `port: 80` 替换在现有 `nacos/` 上是空操作。如果以后有人把 `port: 80` 按两空格缩进写进 yaml，导入时会被改成 8088。
- `linux-shell-sh/`、`test-python/` 没有逐文件执行。检索范围是目录内关键字和全仓引用，可能漏掉注释里的口头约定。

## 9. 候选 OpenSpec capability（只写现状，不建 spec）

本次只交付探查报告，不在 `openspec/specs` 下落文件。

- `local-nacos-import`：把 `nacos/` 灌进本机 Nacos 的 120/998，并改写容器主机名。现状是唯一与 Java `spring.config.import` 同名的导入器；不改 namespace 字面量 `couponkill`；无扩展名的 collaboration 文件不会发布。
- `local-http-smoke`：在 Windows 上拉起 user/gateway/order/coupon，检查登录、订单双前缀和 Long 的 JSON 字符串。现状是不设 `SERVER_PORT`，清理正则与端口归属正则不一致。
- `local-seckill-e2e`：fireDue 预约链、通知冒烟、比价冒烟、铃铛、小规模并发。现状是 fireDue 接受 QUEUED，loadtest 不重置用户去重键，比价冒烟依赖 admin token。
- `ansible-ecs-template`：历史 ECS 剧本。现状是 DEPRECATED、组名和 role 路径对不上、健康检查不是 couponkill 的探针。不应当成部署需求写进 spec，除非单独做删除或归档变更。
- `jmeter-seckill-plan`：2025-09 的压测计划。现状与 `code:0` + `QUEUED` 不一致，CSV 为空。替代品是 `local-loadtest.ps1`。
