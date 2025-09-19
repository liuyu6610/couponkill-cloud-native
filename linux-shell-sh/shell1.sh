#!/bin/bash
# K8s Pod管理与集群检查综合脚本
# 功能：Pod部署验证、故障排查、网络测试、资源清理、系统信息查询

# 确保脚本在错误时退出
set -e

# 配置变量
LOG_DIR="./pod_cleanup_logs"  # 清理日志保存目录
MAX_AGE_DAYS=7                # 清理超过此天数的非运行状态Pod
PING_COUNT=3                  # Ping测试次数

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
NC='\033[0m' # 无颜色

# 日志输出函数
info() { echo -e "${BLUE}[INFO] $*${NC}"; }
success() { echo -e "${GREEN}[SUCCESS] $*${NC}"; }
warning() { echo -e "${YELLOW}[WARNING] $*${NC}"; }
error() { echo -e "${RED}[ERROR] $*${NC}"; }
highlight() { echo -e "${PURPLE}[*] $*${NC}"; }

# 清屏并显示标题
display_header() {
    clear
    echo "============================================="
    echo "        K8s Pod管理与集群检查工具             "
    echo "============================================="
    echo
}

# 等待用户按键继续
press_any_key() {
    echo
    read -n 1 -s -r -p "按任意键继续..."
}

# 1. 部署Pod并验证
deploy_and_verify_pod() {
    display_header
    info "Pod部署与验证工具"
    echo "---------------------------------------------"

    # 检查kubectl是否可用
    if ! command -v kubectl &> /dev/null; then
        error "未安装kubectl，无法执行K8s操作"
        press_any_key
        return 1
    fi

    # 获取部署信息
    read -p "请输入命名空间(默认: default): " NAMESPACE
    NAMESPACE=${NAMESPACE:-default}

    read -p "请选择部署方式 (1=通过YAML文件 2=简单Nginx测试Pod): " DEPLOY_METHOD
    if [ "$DEPLOY_METHOD" = "1" ]; then
        read -p "请输入YAML文件路径: " YAML_FILE
        if [ ! -f "$YAML_FILE" ]; then
            error "文件 $YAML_FILE 不存在"
            press_any_key
            return 1
        fi

        # 部署Pod
        info "开始部署Pod到命名空间 $NAMESPACE..."
        kubectl apply -f "$YAML_FILE" -n "$NAMESPACE"

        # 获取部署的Pod名称
        info "获取部署的Pod信息..."
        POD_NAME=$(kubectl get pods -n "$NAMESPACE" -o jsonpath='{.items[-1].metadata.name}' 2>/dev/null)
        if [ -z "$POD_NAME" ]; then
            warning "无法自动获取Pod名称，请手动输入"
            read -p "请输入部署的Pod名称: " POD_NAME
        fi
    else
        # 创建简单Nginx测试Pod
        read -p "请输入测试Pod名称(默认: test-nginx): " POD_NAME
        POD_NAME=${POD_NAME:-test-nginx}

        info "创建Nginx测试Pod: $POD_NAME 到命名空间 $NAMESPACE..."
        kubectl run "$POD_NAME" --image=nginx:alpine --restart=Never -n "$NAMESPACE"
    fi

    # 等待Pod部署
    echo "---------------------------------------------"
    info "等待Pod就绪，最多等待60秒..."
    if kubectl wait --for=condition=Ready pod/"$POD_NAME" -n "$NAMESPACE" --timeout=60s; then
        success "Pod $POD_NAME 部署成功并已就绪"

        # 显示Pod基本信息
        echo "---------------------------------------------"
        info "Pod基本信息:"
        kubectl get pod "$POD_NAME" -n "$NAMESPACE"

        # 显示Pod详细状态
        echo "---------------------------------------------"
        info "Pod详细状态:"
        kubectl describe pod "$POD_NAME" -n "$NAMESPACE" | grep -A 10 "Status:"
    else
        error "Pod $POD_NAME 部署失败或超时未就绪"
        # 调用故障排查功能
        echo "---------------------------------------------"
        info "自动执行故障排查..."
        analyze_pod_failure "$POD_NAME" "$NAMESPACE"
    fi

    echo "---------------------------------------------"
    success "Pod部署与验证流程完成"
    press_any_key
}

# 2. 分析Pod部署失败原因
analyze_pod_failure() {
    local POD_NAME=$1
    local NAMESPACE=$2

    if [ -z "$POD_NAME" ]; then
        display_header
        info "Pod部署失败分析工具"
        echo "---------------------------------------------"

        read -p "请输入命名空间(默认: default): " NAMESPACE
        NAMESPACE=${NAMESPACE:-default}

        info "命名空间 $NAMESPACE 中的非运行状态Pod:"
        kubectl get pods -n "$NAMESPACE" | grep -v Running | grep -v Completed | grep -v STATUS
        echo

        read -p "请输入要分析的Pod名称: " POD_NAME
        if [ -z "$POD_NAME" ]; then
            error "Pod名称不能为空"
            press_any_key
            return 1
        fi
    else
        echo "---------------------------------------------"
        highlight "分析Pod $POD_NAME (命名空间: $NAMESPACE) 的部署失败原因..."
    fi

    # 检查Pod是否存在
    if ! kubectl get pod "$POD_NAME" -n "$NAMESPACE" &> /dev/null; then
        error "Pod $POD_NAME 在命名空间 $NAMESPACE 中不存在"
        return 1
    fi

    # 显示Pod状态摘要
    echo "---------------------------------------------"
    info "Pod状态摘要:"
    kubectl get pod "$POD_NAME" -n "$NAMESPACE"

    # 显示Pod事件
    echo "---------------------------------------------"
    info "相关事件日志:"
    kubectl describe pod "$POD_NAME" -n "$NAMESPACE" | grep -A 20 "Events:"

    # 检查常见失败原因
    echo "---------------------------------------------"
    info "常见失败原因分析:"

    # 检查镜像拉取问题
    if kubectl describe pod "$POD_NAME" -n "$NAMESPACE" | grep -q "Failed to pull image"; then
        error "发现镜像拉取失败问题"
        kubectl describe pod "$POD_NAME" -n "$NAMESPACE" | grep "Failed to pull image" -A 5
    fi

    # 检查资源不足问题
    if kubectl describe pod "$POD_NAME" -n "$NAMESPACE" | grep -q "Insufficient"; then
        error "发现资源不足问题"
        kubectl describe pod "$POD_NAME" -n "$NAMESPACE" | grep "Insufficient" -A 3
    fi

    # 检查健康检查失败
    if kubectl describe pod "$POD_NAME" -n "$NAMESPACE" | grep -q "Liveness probe failed\|Readiness probe failed"; then
        error "发现健康检查失败问题"
        kubectl describe pod "$POD_NAME" -n "$NAMESPACE" | grep -E "Liveness probe failed|Readiness probe failed" -A 5
    fi

    # 检查权限问题
    if kubectl describe pod "$POD_NAME" -n "$NAMESPACE" | grep -q "permission denied\|unauthorized"; then
        error "发现权限问题"
        kubectl describe pod "$POD_NAME" -n "$NAMESPACE" | grep -E "permission denied|unauthorized" -A 3
    fi

    # 检查容器启动错误
    if kubectl describe pod "$POD_NAME" -n "$NAMESPACE" | grep -q "Error: failed to start container"; then
        error "发现容器启动错误"
        kubectl describe pod "$POD_NAME" -n "$NAMESPACE" | grep "Error: failed to start container" -A 5
    fi

    # 显示Pod日志（如果有）
    echo "---------------------------------------------"
    read -p "是否查看Pod日志? (y/n): " VIEW_LOGS
    if [ "$VIEW_LOGS" = "y" ] || [ "$VIEW_LOGS" = "Y" ]; then
        info "Pod日志:"
        kubectl logs "$POD_NAME" -n "$NAMESPACE" --tail=50 || warning "无法获取Pod日志，可能容器未启动"

        # 检查初始化容器日志
        INIT_CONTAINERS=$(kubectl get pod "$POD_NAME" -n "$NAMESPACE" -o jsonpath='{.spec.initContainers[*].name}')
        if [ -n "$INIT_CONTAINERS" ]; then
            read -p "是否查看初始化容器日志? (y/n): " VIEW_INIT_LOGS
            if [ "$VIEW_INIT_LOGS" = "y" ] || [ "$VIEW_INIT_LOGS" = "Y" ]; then
                for container in $INIT_CONTAINERS; do
                    echo "---------------------------------------------"
                    info "初始化容器 $container 日志:"
                    kubectl logs "$POD_NAME" -n "$NAMESPACE" -c "$container" --tail=50 || warning "无法获取初始化容器 $container 日志"
                done
            fi
        fi
    fi

    echo "---------------------------------------------"
    info "故障分析完成"
}

# 3. 测试Pod网络连通性
test_pod_connectivity() {
    display_header
    info "Pod网络连通性测试工具"
    echo "---------------------------------------------"

    # 检查kubectl是否可用
    if ! command -v kubectl &> /dev/null; then
        error "未安装kubectl，无法执行K8s操作"
        press_any_key
        return 1
    fi

    # 获取测试参数
    read -p "请输入源Pod所在命名空间(默认: default): " SRC_NAMESPACE
    SRC_NAMESPACE=${SRC_NAMESPACE:-default}

    info "命名空间 $SRC_NAMESPACE 中的Pod列表:"
    kubectl get pods -n "$SRC_NAMESPACE"
    echo

    read -p "请输入源Pod名称: " SRC_POD
    if [ -z "$SRC_POD" ]; then
        error "源Pod名称不能为空"
        press_any_key
        return 1
    fi

    # 检查源Pod是否存在且运行中
    if ! kubectl get pod "$SRC_POD" -n "$SRC_NAMESPACE" &> /dev/null; then
        error "源Pod $SRC_POD 在命名空间 $SRC_NAMESPACE 中不存在"
        press_any_key
        return 1
    fi

    if [ "$(kubectl get pod "$SRC_POD" -n "$SRC_NAMESPACE" -o jsonpath='{.status.phase}')" != "Running" ]; then
        warning "源Pod $SRC_POD 不在运行状态，可能影响测试结果"
    fi

    # 选择目标类型
    echo "---------------------------------------------"
    echo "请选择测试目标类型:"
    echo "1. 另一个Pod"
    echo "2. 服务(Service)"
    echo "3. 外部IP地址或域名"
    read -p "请输入选项 (1-3): " TARGET_TYPE

    case $TARGET_TYPE in
        1)
            read -p "请输入目标Pod所在命名空间(默认: default): " DST_NAMESPACE
            DST_NAMESPACE=${DST_NAMESPACE:-default}

            info "命名空间 $DST_NAMESPACE 中的Pod列表:"
            kubectl get pods -n "$DST_NAMESPACE"
            echo

            read -p "请输入目标Pod名称: " DST_POD
            if [ -z "$DST_POD" ]; then
                error "目标Pod名称不能为空"
                press_any_key
                return 1
            fi

            # 获取目标Pod的IP
            DST_IP=$(kubectl get pod "$DST_POD" -n "$DST_NAMESPACE" -o jsonpath='{.status.podIP}')
            if [ -z "$DST_IP" ]; then
                error "无法获取目标Pod $DST_POD 的IP地址"
                press_any_key
                return 1
            fi
            TARGET="$DST_IP ($DST_POD)"
            ;;
        2)
            read -p "请输入目标服务所在命名空间(默认: default): " DST_NAMESPACE
            DST_NAMESPACE=${DST_NAMESPACE:-default}

            info "命名空间 $DST_NAMESPACE 中的服务列表:"
            kubectl get svc -n "$DST_NAMESPACE"
            echo

            read -p "请输入目标服务名称: " DST_SERVICE
            if [ -z "$DST_SERVICE" ]; then
                error "目标服务名称不能为空"
                press_any_key
                return 1
            fi

            TARGET="$DST_SERVICE.$DST_NAMESPACE.svc.cluster.local"
            ;;
        3)
            read -p "请输入目标IP地址或域名: " TARGET
            if [ -z "$TARGET" ]; then
                error "目标IP地址或域名不能为空"
                press_any_key
                return 1
            fi
            ;;
        *)
            error "无效选项"
            press_any_key
            return 1
            ;;
    esac

    # 执行网络测试
    echo "---------------------------------------------"
    info "从Pod $SRC_POD 测试到 $TARGET 的连通性..."

    # Ping测试
    echo "---------------------------------------------"
    info "Ping测试 (共 $PING_COUNT 次):"
    kubectl exec -it "$SRC_POD" -n "$SRC_NAMESPACE" -- ping -c $PING_COUNT "$TARGET" || warning "Ping测试失败"

    # TCP端口测试
    echo "---------------------------------------------"
    read -p "是否进行TCP端口测试? (y/n): " TCP_TEST
    if [ "$TCP_TEST" = "y" ] || [ "$TCP_TEST" = "Y" ]; then
        read -p "请输入要测试的端口号: " PORT
        if [ -n "$PORT" ]; then
            info "测试TCP端口 $PORT 连通性:"
            # 使用telnet或nc测试端口
            if kubectl exec -it "$SRC_POD" -n "$SRC_NAMESPACE" -- sh -c "command -v nc &> /dev/null"; then
                kubectl exec -it "$SRC_POD" -n "$SRC_NAMESPACE" -- nc -zv "$TARGET" "$PORT" || warning "TCP端口 $PORT 测试失败"
            elif kubectl exec -it "$SRC_POD" -n "$SRC_NAMESPACE" -- sh -c "command -v telnet &> /dev/null"; then
                kubectl exec -it "$SRC_POD" -n "$SRC_NAMESPACE" -- timeout 5 telnet "$TARGET" "$PORT" > /dev/null 2>&1 && \
                    success "TCP端口 $PORT 连通" || warning "TCP端口 $PORT 测试失败"
            else
                warning "Pod中未找到nc或telnet工具，无法进行TCP端口测试"
            fi
        else
            warning "端口号不能为空，跳过TCP端口测试"
        fi
    fi

    # DNS解析测试（如果目标是域名）
    if [[ "$TARGET" == *.* && ! "$TARGET" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        echo "---------------------------------------------"
        read -p "是否进行DNS解析测试? (y/n): " DNS_TEST
        if [ "$DNS_TEST" = "y" ] || [ "$DNS_TEST" = "Y" ]; then
            info "测试DNS解析 $TARGET:"
            kubectl exec -it "$SRC_POD" -n "$SRC_NAMESPACE" -- nslookup "$TARGET" || warning "DNS解析测试失败"
        fi
    fi

    echo "---------------------------------------------"
    success "网络连通性测试完成"
    press_any_key
}

# 4. 清理非运行状态的Pod
cleanup_non_running_pods() {
    display_header
    info "非运行状态Pod清理工具"
    echo "---------------------------------------------"

    # 检查kubectl是否可用
    if ! command -v kubectl &> /dev/null; then
        error "未安装kubectl，无法执行K8s操作"
        press_any_key
        return 1
    fi

    # 创建日志目录
    TODAY=$(date +%Y-%m-%d)
    LOG_FILE="$LOG_DIR/$TODAY/pod_cleanup.log"
    mkdir -p "$(dirname "$LOG_FILE")"

    # 记录清理开始时间
    echo "===== 清理开始于: $(date) =====" >> "$LOG_FILE"

    # 选择清理范围
    echo "请选择清理范围:"
    echo "1. 所有命名空间"
    echo "2. 指定命名空间"
    read -p "请输入选项 (1-2): " SCOPE_CHOICE

    case $SCOPE_CHOICE in
        1)
            NAMESPACES=$(kubectl get namespaces -o jsonpath='{.items[*].metadata.name}')
            FILTER="--all-namespaces"
            ;;
        2)
            read -p "请输入要清理的命名空间: " NAMESPACE
            if [ -z "$NAMESPACE" ]; then
                error "命名空间不能为空"
                press_any_key
                return 1
            fi
            if ! kubectl get namespace "$NAMESPACE" &> /dev/null; then
                error "命名空间 $NAMESPACE 不存在"
                press_any_key
                return 1
            fi
            NAMESPACES="$NAMESPACE"
            FILTER="-n $NAMESPACE"
            ;;
        *)
            error "无效选项"
            press_any_key
            return 1
            ;;
    esac

    # 选择清理条件
    echo "---------------------------------------------"
    echo "请选择清理条件:"
    echo "1. 所有非运行状态的Pod (Failed, Succeeded, Unknown)"
    echo "2. 仅清理超过 $MAX_AGE_DAYS 天的非运行状态Pod"
    read -p "请输入选项 (1-2): " CONDITION_CHOICE

    # 查找符合条件的Pod
    info "正在查找符合条件的Pod..."
    NON_RUNNING_PODS=()

    for ns in $NAMESPACES; do
        # 获取所有非运行状态的Pod
        pods=$(kubectl get pods -n "$ns" --no-headers 2>/dev/null | grep -v Running | awk '{print $1 " " $3}')
        if [ -n "$pods" ]; then
            while read -r pod status; do
                # 检查Pod年龄
                if [ "$CONDITION_CHOICE" = "2" ]; then
                    # 获取Pod创建时间
                    creation_time=$(kubectl get pod "$pod" -n "$ns" -o jsonpath='{.metadata.creationTimestamp}')
                    # 转换为时间戳
                    creation_timestamp=$(date -d "$creation_time" +%s)
                    current_timestamp=$(date +%s)
                    age_days=$(( (current_timestamp - creation_timestamp) / 86400 ))

                    if [ $age_days -ge $MAX_AGE_DAYS ]; then
                        NON_RUNNING_PODS+=("$ns:$pod:$status:$age_days天")
                    fi
                else
                    NON_RUNNING_PODS+=("$ns:$pod:$status")
                fi
            done <<< "$pods"
        fi
    done

    # 显示找到的Pod
    echo "---------------------------------------------"
    if [ ${#NON_RUNNING_PODS[@]} -eq 0 ]; then
        success "未找到符合条件的非运行状态Pod"
        press_any_key
        return 0
    fi

    info "找到 ${#NON_RUNNING_PODS[@]} 个符合条件的非运行状态Pod:"
    printf "%-20s %-30s %-10s %s\n" "命名空间" "Pod名称" "状态" "年龄(天)"
    echo "-------------------------------------------------------------------------"
    for pod_info in "${NON_RUNNING_PODS[@]}"; do
        IFS=':' read -r ns pod status age <<< "$pod_info"
        printf "%-20s %-30s %-10s %s\n" "$ns" "$pod" "$status" "${age:-N/A}"
    done

    # 确认清理
    echo "---------------------------------------------"
    read -p "是否要删除这些Pod? (y/n): " CONFIRM
    if [ "$CONFIRM" != "y" ] && [ "$CONFIRM" != "Y" ]; then
        info "已取消清理操作"
        press_any_key
        return 0
    fi

    # 执行清理并记录信息
    info "开始清理Pod..."
    echo "清理的Pod列表:" >> "$LOG_FILE"
    printf "%-20s %-30s %-10s %s\n" "命名空间" "Pod名称" "状态" "年龄(天)" >> "$LOG_FILE"
    echo "-------------------------------------------------------------------------" >> "$LOG_FILE"

    for pod_info in "${NON_RUNNING_PODS[@]}"; do
        IFS=':' read -r ns pod status age <<< "$pod_info"
        printf "%-20s %-30s %-10s %s\n" "$ns" "$pod" "$status" "${age:-N/A}" >> "$LOG_FILE"

        # 记录Pod详细信息
        echo -e "\n===== Pod $pod 详细信息 =====" >> "$LOG_FILE"
        kubectl describe pod "$pod" -n "$ns" >> "$LOG_FILE" 2>&1

        # 删除Pod
        info "删除Pod $pod (命名空间: $ns)..."
        if kubectl delete pod "$pod" -n "$ns" &> /dev/null; then
            success "成功删除Pod $pod"
            echo "删除时间: $(date)" >> "$LOG_FILE"
        else
            error "删除Pod $pod 失败"
            echo "删除失败时间: $(date)" >> "$LOG_FILE"
        fi
    done

    # 记录清理结束时间
    echo -e "\n===== 清理结束于: $(date) =====" >> "$LOG_FILE"

    echo "---------------------------------------------"
    success "清理完成，共处理 ${#NON_RUNNING_PODS[@]} 个Pod"
    info "清理日志已保存至: $LOG_FILE"
    press_any_key
}

# 5. Linux系统信息查询（端口、进程等）
linux_system_info() {
    display_header
    info "Linux系统信息查询工具"
    echo "---------------------------------------------"

    while true; do
        echo "请选择查询类型:"
        echo "1. 查找占用特定端口的进程"
        echo "2. 查找特定名称的进程"
        echo "3. 查看系统资源使用情况"
        echo "4. 查看网络连接状态"
        echo "5. 返回主菜单"
        echo
        read -p "请输入选项 (1-5): " CHOICE

        case $CHOICE in
            1)
                # 查找占用特定端口的进程
                read -p "请输入要查询的端口号: " PORT
                if [ -z "$PORT" ]; then
                    error "端口号不能为空"
                    echo
                    continue
                fi

                echo "---------------------------------------------"
                info "查找占用端口 $PORT 的进程:"

                # 使用lsof或netstat查找端口
                if command -v lsof &> /dev/null; then
                    lsof -i :$PORT
                elif command -v netstat &> /dev/null; then
                    netstat -tulnp | grep ":$PORT"
                elif command -v ss &> /dev/null; then
                    ss -tulnp | grep ":$PORT"
                else
                    error "未找到lsof、netstat或ss工具，无法查询端口信息"
                fi
                echo "---------------------------------------------"
                press_any_key
                display_header
                ;;
            2)
                # 查找特定名称的进程
                read -p "请输入要查询的进程名称或关键词: " PROCESS_NAME
                if [ -z "$PROCESS_NAME" ]; then
                    error "进程名称不能为空"
                    echo
                    continue
                fi

                echo "---------------------------------------------"
                info "查找名称包含 '$PROCESS_NAME' 的进程:"
                ps aux | grep "$PROCESS_NAME" | grep -v grep

                echo "---------------------------------------------"
                read -p "是否要查看其中某个进程的详细信息? (y/n): " VIEW_DETAILS
                if [ "$VIEW_DETAILS" = "y" ] || [ "$VIEW_DETAILS" = "Y" ]; then
                    read -p "请输入进程ID (PID): " PID
                    if [ -n "$PID" ] && [ "$PID" -eq "$PID" ] 2>/dev/null; then
                        echo "---------------------------------------------"
                        info "进程 $PID 的详细信息:"
                        ps aux | grep "$PID" | grep -v grep
                        echo
                        info "进程 $PID 的打开文件:"
                        if command -v lsof &> /dev/null; then
                            lsof -p "$PID" | head -n 10
                            echo "显示前10个文件，共 $(lsof -p "$PID" | wc -l) 个"
                        else
                            warning "未安装lsof，无法查看进程打开的文件"
                        fi
                    else
                        error "无效的进程ID"
                    fi
                fi
                echo "---------------------------------------------"
                press_any_key
                display_header
                ;;
            3)
                # 查看系统资源使用情况
                echo "---------------------------------------------"
                info "CPU使用情况 (按q退出):"
                top -b -n 1 | head -n 15

                echo "---------------------------------------------"
                info "内存使用情况:"
                free -h

                echo "---------------------------------------------"
                info "磁盘使用情况:"
                df -h | grep -v tmpfs | grep -v loop

                echo "---------------------------------------------"
                info "系统负载:"
                uptime
                press_any_key
                display_header
                ;;
            4)
                # 查看网络连接状态
                echo "---------------------------------------------"
                info "TCP连接状态摘要:"
                netstat -tuln | awk '/^tcp/ {split($4, a, ":"); port = a[length(a)]; count[port]++} END {for (p in count) print p, count[p]}' | sort -nr -k2

                echo "---------------------------------------------"
                info "活跃网络连接 (前20条):"
                netstat -tulnp | head -n 20

                echo "---------------------------------------------"
                info "网络接口流量:"
                if command -v iftop &> /dev/null; then
                    echo "使用iftop查看实时流量 (按q退出):"
                    iftop -n -t -s 10
                else
                    warning "未安装iftop，显示基本接口信息:"
                    ip -s link show | grep -A 2 "link/"
                fi
                press_any_key
                display_header
                ;;
            5)
                # 返回主菜单
                break
                ;;
            *)
                error "无效选项，请输入1-5之间的数字"
                echo
                ;;
        esac
    done
}

# 主菜单
main_menu() {
    while true; do
        display_header
        echo "请选择要执行的操作:"
        echo "1. 部署Pod并验证"
        echo "2. 分析Pod部署失败原因"
        echo "3. 测试Pod网络连通性"
        echo "4. 清理非运行状态的Pod"
        echo "5. Linux系统信息查询 (端口、进程等)"
        echo "0. 退出脚本"
        echo
        read -p "请输入选项 (0-5): " CHOICE

        case $CHOICE in
            1) deploy_and_verify_pod ;;
            2) analyze_pod_failure ;;
            3) test_pod_connectivity ;;
            4) cleanup_non_running_pods ;;
            5) linux_system_info ;;
            0)
                display_header
                echo "感谢使用K8s Pod管理与集群检查工具，再见！"
                echo
                exit 0
                ;;
            *)
                warning "无效选项，请输入0-5之间的数字"
                press_any_key
                ;;
        esac
    done
}

# 启动主菜单
main_menu