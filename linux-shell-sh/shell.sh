#!/bin/bash
# 系统与K8s集群综合排查脚本
# 支持交互式选择排查项，涵盖Linux系统与K8s集群的常见问题检查

# 确保脚本在错误时退出
set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # 无颜色

# 日志输出函数
info() { echo -e "${BLUE}[INFO] $*${NC}"; }
success() { echo -e "${GREEN}[SUCCESS] $*${NC}"; }
warning() { echo -e "${YELLOW}[WARNING] $*${NC}"; }
error() { echo -e "${RED}[ERROR] $*${NC}"; }

# 清屏并显示标题
display_header() {
    clear
    echo "============================================="
    echo "          系统与K8s集群排查工具               "
    echo "============================================="
    echo
}

# 1. 检查Linux磁盘使用情况
check_disk() {
    display_header
    info "开始检查磁盘使用情况..."
    echo "---------------------------------------------"

    # 显示磁盘整体使用情况
    df -h | grep -v tmpfs | grep -v loop | while read line; do
        USED_PERCENT=$(echo $line | awk '{print $5}' | sed 's/%//')
        MOUNT_POINT=$(echo $line | awk '{print $6}')

        if [ $USED_PERCENT -ge 90 ]; then
            echo -e "${RED}$line${NC}"
            warning "磁盘 $MOUNT_POINT 使用率超过90%，需要立即处理！"
        elif [ $USED_PERCENT -ge 80 ]; then
            echo -e "${YELLOW}$line${NC}"
            warning "磁盘 $MOUNT_POINT 使用率超过80%，请注意监控"
        else
            echo "$line"
        fi
    done

    echo "---------------------------------------------"
    info "检查磁盘I/O负载..."
    iostat -x 2 1 | grep -v ^avg-cpu | grep -v ^Device | grep -v ^$

    echo "---------------------------------------------"
    info "检查inode使用情况..."
    df -i | grep -v tmpfs | grep -v loop

    echo "---------------------------------------------"
    success "磁盘检查完成"
    press_any_key
}

# 2. 检查Linux CPU使用情况
check_cpu() {
    display_header
    info "开始检查CPU使用情况..."
    echo "---------------------------------------------"

    # 显示CPU整体使用情况
    info "CPU整体使用率:"
    top -bn1 | grep "Cpu(s)" | awk '{printf "用户态: %.2f%%, 系统态: %.2f%%, 空闲: %.2f%%\n", $2, $4, $8}'

    echo "---------------------------------------------"
    info "CPU核心数与负载:"
    echo "物理核心数: $(grep 'physical id' /proc/cpuinfo | sort -u | wc -l)"
    echo "逻辑核心数: $(grep 'processor' /proc/cpuinfo | wc -l)"
    echo "负载平均值: $(uptime | awk -F'load average: ' '{print $2}')"

    echo "---------------------------------------------"
    info "占用CPU最高的10个进程:"
    ps -eo %cpu,%mem,user,pid,cmd --sort=-%cpu | head -n 11

    echo "---------------------------------------------"
    success "CPU检查完成"
    press_any_key
}

# 3. 检查Linux内存使用情况
check_memory() {
    display_header
    info "开始检查内存使用情况..."
    echo "---------------------------------------------"

    # 显示内存整体使用情况
    info "内存使用概况:"
    free -h

    echo "---------------------------------------------"
    info "内存详细使用:"
    cat /proc/meminfo | grep -E 'MemTotal|MemFree|MemAvailable|Buffers|Cached|SwapTotal|SwapFree'

    echo "---------------------------------------------"
    info "占用内存最高的10个进程:"
    ps -eo %mem,%cpu,user,pid,cmd --sort=-%mem | head -n 11

    echo "---------------------------------------------"
    success "内存检查完成"
    press_any_key
}

# 4. 检查Linux网络情况
check_network() {
    display_header
    info "开始检查网络情况..."
    echo "---------------------------------------------"

    # 显示网络接口与IP
    info "网络接口与IP地址:"
    ip addr show | grep -E '^[0-9]|inet '

    echo "---------------------------------------------"
    info "网络连接状态:"
    netstat -tuln

    echo "---------------------------------------------"
    info "网络路由表:"
    ip route show

    echo "---------------------------------------------"
    info "DNS配置:"
    cat /etc/resolv.conf | grep -v '^#' | grep -v '^$'

    echo "---------------------------------------------"
    info "网络连通性测试(默认网关):"
    GATEWAY=$(ip route show default | awk '/default/ {print $3}')
    if [ -n "$GATEWAY" ]; then
        ping -c 3 $GATEWAY
    else
        warning "未找到默认网关"
    fi

    echo "---------------------------------------------"
    success "网络检查完成"
    press_any_key
}

# 5. 检查Linux系统服务状态
check_services() {
    display_header
    info "开始检查系统服务状态..."
    echo "---------------------------------------------"

    # 显示系统状态
    info "系统基本信息:"
    uname -a
    echo "系统版本: $(cat /etc/os-release | grep PRETTY_NAME | cut -d= -f2 | sed 's/"//g')"
    echo "系统运行时间: $(uptime | awk '{print $3 " " $4}' | sed 's/,//')"

    echo "---------------------------------------------"
    info "关键系统服务状态:"
    SERVICES=("sshd" "docker" "kubelet" "firewalld" "NetworkManager")
    for service in "${SERVICES[@]}"; do
        if systemctl is-active --quiet $service; then
            echo -e "$service: ${GREEN}运行中${NC}"
        else
            echo -e "$service: ${RED}未运行${NC}"
        fi
    done

    echo "---------------------------------------------"
    info "最近5条系统错误日志:"
    journalctl -p err --no-pager | tail -n 5

    echo "---------------------------------------------"
    success "系统服务检查完成"
    press_any_key
}

# 6. 检查K8s集群组件状态
check_k8s_components() {
    display_header
    info "开始检查K8s集群组件状态..."
    echo "---------------------------------------------"

    # 检查kubectl是否可用
    if ! command -v kubectl &> /dev/null; then
        error "未安装kubectl，无法检查K8s集群"
        press_any_key
        return 1
    fi

    # 检查集群信息
    info "集群基本信息:"
    kubectl cluster-info

    echo "---------------------------------------------"
    info "集群版本信息:"
    kubectl version --short

    echo "---------------------------------------------"
    info "控制平面组件状态:"
    kubectl get pods -n kube-system -l 'component in (kube-apiserver,kube-controller-manager,kube-scheduler,etcd)'

    echo "---------------------------------------------"
    info "核心插件状态:"
    kubectl get pods -n kube-system -l 'k8s-app in (kube-dns,coredns,calico,kube-proxy)'

    echo "---------------------------------------------"
    success "K8s组件检查完成"
    press_any_key
}

# 7. 检查K8s节点状态
check_k8s_nodes() {
    display_header
    info "开始检查K8s节点状态..."
    echo "---------------------------------------------"

    # 检查kubectl是否可用
    if ! command -v kubectl &> /dev/null; then
        error "未安装kubectl，无法检查K8s集群"
        press_any_key
        return 1
    fi

    # 显示节点状态
    info "节点状态概览:"
    kubectl get nodes

    echo "---------------------------------------------"
    info "节点详细状态:"
    kubectl describe nodes | grep -A 10 "Conditions:" | grep -v "Type:" | grep -v "Status:" | grep -v "LastHeartbeatTime:"

    echo "---------------------------------------------"
    info "节点资源使用情况:"
    kubectl top nodes

    echo "---------------------------------------------"
    success "K8s节点检查完成"
    press_any_key
}

# 8. 检查K8s命名空间与Pod状态
check_k8s_namespaces_pods() {
    display_header
    info "开始检查K8s命名空间与Pod状态..."
    echo "---------------------------------------------"

    # 检查kubectl是否可用
    if ! command -v kubectl &> /dev/null; then
        error "未安装kubectl，无法检查K8s集群"
        press_any_key
        return 1
    fi

    # 选择要检查的命名空间
    info "可用命名空间:"
    kubectl get namespaces -o jsonpath='{range .items[*]}{.metadata.name}{" "}{end}'
    echo -e "\n"
    read -p "请输入要检查的命名空间(默认: default): " NAMESPACE
    NAMESPACE=${NAMESPACE:-default}

    echo "---------------------------------------------"
    info "命名空间 $NAMESPACE 中的Pod状态:"
    kubectl get pods -n $NAMESPACE

    echo "---------------------------------------------"
    info "状态异常的Pod详细信息:"
    NOT_RUNNING=$(kubectl get pods -n $NAMESPACE | grep -v Running | grep -v Completed | grep -v STATUS | awk '{print $1}')
    if [ -n "$NOT_RUNNING" ]; then
        for pod in $NOT_RUNNING; do
            echo -e "\n${YELLOW}Pod: $pod 详细信息:${NC}"
            kubectl describe pod $pod -n $NAMESPACE | grep -A 10 "Status:" | grep -A 10 "Events:"
        done
    else
        success "命名空间 $NAMESPACE 中所有Pod均正常运行"
    fi

    echo "---------------------------------------------"
    info "Pod资源使用情况:"
    kubectl top pods -n $NAMESPACE || warning "无法获取Pod资源使用情况，可能未安装metrics-server"

    echo "---------------------------------------------"
    success "K8s命名空间与Pod检查完成"
    press_any_key
}

# 9. 检查K8s服务与Ingress状态
check_k8s_services_ingress() {
    display_header
    info "开始检查K8s服务与Ingress状态..."
    echo "---------------------------------------------"

    # 检查kubectl是否可用
    if ! command -v kubectl &> /dev/null; then
        error "未安装kubectl，无法检查K8s集群"
        press_any_key
        return 1
    fi

    # 选择要检查的命名空间
    read -p "请输入要检查的命名空间(默认: default): " NAMESPACE
    NAMESPACE=${NAMESPACE:-default}

    echo "---------------------------------------------"
    info "命名空间 $NAMESPACE 中的Service:"
    kubectl get services -n $NAMESPACE

    echo "---------------------------------------------"
    info "命名空间 $NAMESPACE 中的Ingress:"
    if kubectl get ingress -n $NAMESPACE &> /dev/null; then
        kubectl get ingress -n $NAMESPACE
    else
        warning "命名空间 $NAMESPACE 中没有Ingress资源或Ingress控制器未安装"
    fi

    echo "---------------------------------------------"
    read -p "是否要测试某个Service的连通性? (y/n): " TEST_SERVICE
    if [ "$TEST_SERVICE" = "y" ] || [ "$TEST_SERVICE" = "Y" ]; then
        read -p "请输入要测试的Service名称: " SERVICE_NAME
        read -p "请输入Service端口: " SERVICE_PORT
        info "测试Service $SERVICE_NAME:$SERVICE_PORT 的连通性..."
        kubectl run -it --rm test-service --image=busybox:1.35 --restart=Never -- \
            sh -c "wget -qO- $SERVICE_NAME:$SERVICE_PORT && echo -e '\n${GREEN}Service访问成功${NC}' || echo -e '\n${RED}Service访问失败${NC}'"
    fi

    echo "---------------------------------------------"
    success "K8s服务与Ingress检查完成"
    press_any_key
}

# 10. 检查K8s事件与日志
check_k8s_events() {
    display_header
    info "开始检查K8s事件与日志..."
    echo "---------------------------------------------"

    # 检查kubectl是否可用
    if ! command -v kubectl &> /dev/null; then
        error "未安装kubectl，无法检查K8s集群"
        press_any_key
        return 1
    fi

    # 选择要检查的命名空间
    read -p "请输入要检查的命名空间(默认: default): " NAMESPACE
    NAMESPACE=${NAMESPACE:-default}

    echo "---------------------------------------------"
    info "命名空间 $NAMESPACE 中最近的30条事件:"
    kubectl get events -n $NAMESPACE --sort-by='.lastTimestamp' | tail -n 30

    echo "---------------------------------------------"
    read -p "是否要查看某个Pod的日志? (y/n): " VIEW_LOGS
    if [ "$VIEW_LOGS" = "y" ] || [ "$VIEW_LOGS" = "Y" ]; then
        info "命名空间 $NAMESPACE 中的Pod列表:"
        kubectl get pods -n $NAMESPACE
        read -p "请输入要查看日志的Pod名称: " POD_NAME
        read -p "请输入要显示的日志行数(默认: 100): " LOG_LINES
        LOG_LINES=${LOG_LINES:-100}
        read -p "是否要搜索特定关键词? (输入关键词，直接回车跳过): " LOG_KEYWORD

        info "显示Pod $POD_NAME 的最后 $LOG_LINES 行日志:"
        if [ -n "$LOG_KEYWORD" ]; then
            kubectl logs $POD_NAME -n $NAMESPACE --tail=$LOG_LINES | grep "$LOG_KEYWORD" --color
        else
            kubectl logs $POD_NAME -n $NAMESPACE --tail=$LOG_LINES
        fi
    fi

    echo "---------------------------------------------"
    success "K8s事件与日志检查完成"
    press_any_key
}

# 等待用户按键继续
press_any_key() {
    echo
    read -n 1 -s -r -p "按任意键继续..."
}

# 主菜单
main_menu() {
    while true; do
        display_header
        echo "请选择要执行的检查:"
        echo "1. 检查Linux磁盘使用情况"
        echo "2. 检查Linux CPU使用情况"
        echo "3. 检查Linux内存使用情况"
        echo "4. 检查Linux网络情况"
        echo "5. 检查Linux系统服务状态"
        echo "6. 检查K8s集群组件状态"
        echo "7. 检查K8s节点状态"
        echo "8. 检查K8s命名空间与Pod状态"
        echo "9. 检查K8s服务与Ingress状态"
        echo "10. 检查K8s事件与日志"
        echo "0. 退出脚本"
        echo
        read -p "请输入选项 (0-10): " CHOICE

        case $CHOICE in
            1) check_disk ;;
            2) check_cpu ;;
            3) check_memory ;;
            4) check_network ;;
            5) check_services ;;
            6) check_k8s_components ;;
            7) check_k8s_nodes ;;
            8) check_k8s_namespaces_pods ;;
            9) check_k8s_services_ingress ;;
            10) check_k8s_events ;;
            0)
                display_header
                echo "感谢使用系统与K8s集群排查工具，再见！"
                echo
                exit 0
                ;;
            *)
                warning "无效选项，请输入0-10之间的数字"
                press_any_key
                ;;
        esac
    done
}

# 启动主菜单
main_menu
