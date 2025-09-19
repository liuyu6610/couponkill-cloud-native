#!/bin/bash
# 增强版Linux系统开发环境一键配置脚本
# 功能：查询并安装常见编程语言、云原生组件及开发工具
# 支持：子组件独立选择安装、类别一键安装

# 确保脚本以root权限运行
if [ "$(id -u)" -ne 0 ]; then
    echo "请使用root权限运行此脚本 (sudo $0)" >&2
    exit 1
fi

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

# 检测系统类型
detect_os() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS=$NAME
        VERSION=$VERSION_ID

        if [[ $OS == *"Ubuntu"* || $OS == *"Debian"* ]]; then
            OS_TYPE="debian"
            info "检测到 Debian/Ubuntu 系统: $OS $VERSION"
        elif [[ $OS == *"CentOS"* || $OS == *"Red Hat"* || $OS == *"Rocky"* || $OS == *"AlmaLinux"* ]]; then
            OS_TYPE="rhel"
            info "检测到 RHEL/CentOS 系统: $OS $VERSION"
        else
            error "不支持的操作系统: $OS"
            exit 1
        fi
    else
        error "无法检测操作系统类型"
        exit 1
    fi
}

# 初始化包管理器
initialize_package_manager() {
    info "初始化包管理器..."

    if [ "$OS_TYPE" = "debian" ]; then
        apt update -qq > /dev/null
        apt install -qq -y apt-transport-https ca-certificates curl software-properties-common > /dev/null
    else
        yum install -qq -y epel-release > /dev/null
        yum update -qq -y > /dev/null
    fi

    success "包管理器初始化完成"
}

# 检查软件是否已安装
is_installed() {
    local cmd=$1
    if command -v "$cmd" &> /dev/null; then
        return 0  # 已安装
    else
        return 1  # 未安装
    fi
}

# 刷新环境变量
refresh_environment() {
    info "刷新环境变量..."
    # 尝试重新加载bash配置
    if [ -f "$HOME/.bashrc" ]; then
        source "$HOME/.bashrc"
    fi
    if [ -f "$HOME/.bash_profile" ]; then
        source "$HOME/.bash_profile"
    fi
    if [ -f "/etc/profile" ]; then
        source "/etc/profile"
    fi
    success "环境变量已刷新"
}

# 安装函数 - Debian/Ubuntu
install_debian() {
    local pkg=$1
    info "安装 $pkg..."
    if apt install -qq -y "$pkg" > /dev/null; then
        success "$pkg 安装成功"
        return 0
    else
        error "$pkg 安装失败"
        return 1
    fi
}

# 安装函数 - RHEL/CentOS
install_rhel() {
    local pkg=$1
    info "安装 $pkg..."
    if yum install -qq -y "$pkg" > /dev/null; then
        success "$pkg 安装成功"
        return 0
    else
        error "$pkg 安装失败"
        return 1
    fi
}

# 通用安装函数
install_package() {
    local pkg_name=$1
    local debian_pkg=${2:-$1}
    local rhel_pkg=${3:-$1}

    if [ "$OS_TYPE" = "debian" ]; then
        install_debian "$debian_pkg"
    else
        install_rhel "$rhel_pkg"
    fi
}

# 编程语言安装函数（支持单独调用）
install_python() {
    highlight "处理 Python 环境"

    if is_installed python3; then
        info "Python3 已安装: $(python3 --version 2>&1)"
    else
        read -p "是否安装 Python3? (y/n): " choice
        if [ "$choice" = "y" ] || [ "$choice" = "Y" ]; then
            install_package "python3"
            install_package "python3-pip"
            # 设置pip3为默认pip
            ln -s /usr/bin/pip3 /usr/bin/pip 2>/dev/null
            success "Python3 环境配置完成"
        fi
    fi

    if is_installed python3-pip; then
        info "pip3 已安装: $(pip3 --version 2>&1 | head -n1)"
    else
        read -p "是否安装 pip3? (y/n): " choice
        if [ "$choice" = "y" ] || [ "$choice" = "Y" ]; then
            install_package "python3-pip"
            ln -s /usr/bin/pip3 /usr/bin/pip 2>/dev/null
        fi
    fi
}

install_java() {
    highlight "处理 Java 环境"

    if is_installed java; then
        info "Java 已安装: $(java -version 2>&1 | head -n1)"
        return
    fi

    read -p "是否安装 OpenJDK? (y/n): " choice
    if [ "$choice" = "y" ] || [ "$choice" = "Y" ]; then
        read -p "请选择Java版本 (8/11/17，默认11): " java_version
        java_version=${java_version:-11}

        if [ "$OS_TYPE" = "debian" ]; then
            install_package "openjdk-$java_version-jdk"
        else
            install_package "java-$java_version-openjdk-devel"
        fi

        # 配置环境变量
        echo "export JAVA_HOME=$(dirname $(dirname $(readlink $(readlink $(which java)))))" >> /etc/profile
        echo "export PATH=\$JAVA_HOME/bin:\$PATH" >> /etc/profile
        refresh_environment

        if is_installed java; then
            success "OpenJDK $java_version 安装完成: $(java -version 2>&1 | head -n1)"
        fi
    fi
}

install_nodejs() {
    highlight "处理 Node.js 环境"

    if is_installed node; then
        info "Node.js 已安装: $(node --version)"
        info "npm 已安装: $(npm --version)"
        return
    fi

    read -p "是否安装 Node.js? (y/n): " choice
    if [ "$choice" = "y" ] || [ "$choice" = "Y" ]; then
        read -p "请选择Node.js主版本 (16/18/20，默认18): " node_version
        node_version=${node_version:-18}

        # 使用NodeSource安装
        curl -fsSL https://deb.nodesource.com/setup_$node_version.x | bash - > /dev/null 2>&1
        install_package "nodejs"

        # 安装yarn
        read -p "是否安装 yarn? (y/n): " install_yarn
        if [ "$install_yarn" = "y" ] || [ "$install_yarn" = "Y" ]; then
            npm install -g yarn > /dev/null 2>&1
            success "yarn 安装完成: $(yarn --version)"
        fi

        if is_installed node; then
            success "Node.js $node_version 安装完成: $(node --version)"
            success "npm 安装完成: $(npm --version)"
        fi
    fi
}

install_go() {
    highlight "处理 Go 环境"

    if is_installed go; then
        info "Go 已安装: $(go version)"
        return
    fi

    read -p "是否安装 Go? (y/n): " choice
    if [ "$choice" = "y" ] || [ "$choice" = "Y" ]; then
        read -p "请输入Go版本 (例如 1.21.4，默认1.21.4): " go_version
        go_version=${go_version:-1.21.4}

        # 下载并安装Go
        GO_TAR="go$go_version.linux-amd64.tar.gz"
        wget -q "https://dl.google.com/go/$GO_TAR" -O "/tmp/$GO_TAR"

        # 移除旧版本并安装新版本
        rm -rf /usr/local/go
        tar -C /usr/local -xzf "/tmp/$GO_TAR" > /dev/null
        rm "/tmp/$GO_TAR"

        # 配置环境变量
        echo "export GOROOT=/usr/local/go" >> /etc/profile
        echo "export GOPATH=\$HOME/go" >> /etc/profile
        echo "export PATH=\$GOROOT/bin:\$GOPATH/bin:\$PATH" >> /etc/profile
        refresh_environment

        # 创建GOPATH目录
        mkdir -p "$HOME/go"

        if is_installed go; then
            success "Go $go_version 安装完成: $(go version)"
        fi
    fi
}

# 容器与Kubernetes工具安装函数（支持单独调用）
install_docker() {
    highlight "处理 Docker 环境"

    if is_installed docker; then
        info "Docker 已安装: $(docker --version)"
        return
    fi

    read -p "是否安装 Docker? (y/n): " choice
    if [ "$choice" = "y" ] || [ "$choice" = "Y" ]; then
        # 安装依赖
        if [ "$OS_TYPE" = "debian" ]; then
            install_package "docker-ce" "" ""

            # 添加Docker官方GPG密钥
            curl -fsSL https://download.docker.com/linux/ubuntu/gpg | apt-key add - > /dev/null 2>&1

            # 添加Docker仓库
            add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" > /dev/null 2>&1
            apt update -qq > /dev/null
        else
            # RHEL/CentOS安装Docker
            yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo > /dev/null 2>&1
            install_package "docker-ce"
        fi

        # 启动并设置开机自启
        systemctl start docker
        systemctl enable docker > /dev/null

        # 将当前用户添加到docker组
        read -p "是否将当前用户添加到docker组? (y/n): " add_user
        if [ "$add_user" = "y" ] || [ "$add_user" = "Y" ]; then
            usermod -aG docker "$SUDO_USER"
            warning "用户已添加到docker组，需重新登录生效"
        fi

        if is_installed docker; then
            success "Docker 安装完成: $(docker --version)"
        fi
    fi
}

install_kubectl() {
    highlight "处理 kubectl 工具"

    if is_installed kubectl; then
        info "kubectl 已安装: $(kubectl version --client --short)"
        return
    fi

    read -p "是否安装 kubectl? (y/n): " choice
    if [ "$choice" = "y" ] || [ "$choice" = "Y" ]; then
        # 下载kubectl
        curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" > /dev/null 2>&1
        chmod +x kubectl
        mv kubectl /usr/local/bin/

        if is_installed kubectl; then
            success "kubectl 安装完成: $(kubectl version --client --short)"
        fi
    fi
}

install_helm() {
    highlight "处理 Helm 工具"

    if is_installed helm; then
        info "Helm 已安装: $(helm version --short)"
        return
    fi

    read -p "是否安装 Helm? (y/n): " choice
    if [ "$choice" = "y" ] || [ "$choice" = "Y" ]; then
        # 下载Helm
        curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 > /dev/null 2>&1
        chmod 700 get_helm.sh
        ./get_helm.sh > /dev/null 2>&1
        rm get_helm.sh

        if is_installed helm; then
            success "Helm 安装完成: $(helm version --short)"
        fi
    fi
}

install_minikube() {
    highlight "处理 Minikube 工具"

    if is_installed minikube; then
        info "Minikube 已安装: $(minikube version)"
        return
    fi

    read -p "是否安装 Minikube? (y/n): " choice
    if [ "$choice" = "y" ] || [ "$choice" = "Y" ]; then
        # 下载Minikube
        curl -LO https://storage.googleapis.com/minikube/releases/latest/minikube-linux-amd64 > /dev/null 2>&1
        install -o root -g root -m 0755 minikube-linux-amd64 /usr/local/bin/minikube
        rm minikube-linux-amd64

        if is_installed minikube; then
            success "Minikube 安装完成: $(minikube version)"
        fi
    fi
}

# 云原生工具安装函数（支持单独调用）
install_terraform() {
    highlight "处理 Terraform 工具"

    if is_installed terraform; then
        info "terraform 已安装: $(terraform --version | head -n1)"
        return
    fi

    read -p "是否安装 Terraform? (y/n): " choice
    if [ "$choice" = "y" ] || [ "$choice" = "Y" ]; then
        if [ "$OS_TYPE" = "debian" ]; then
            curl -fsSL https://apt.releases.hashicorp.com/gpg | apt-key add - > /dev/null 2>&1
            apt-add-repository "deb [arch=amd64] https://apt.releases.hashicorp.com $(lsb_release -cs) main" > /dev/null 2>&1
            apt update -qq > /dev/null
        else
            yum install -qq -y yum-utils > /dev/null
            yum-config-manager --add-repo https://rpm.releases.hashicorp.com/RHEL/hashicorp.repo > /dev/null 2>&1
        fi
        install_package "terraform"

        if is_installed terraform; then
            success "Terraform 安装完成: $(terraform --version | head -n1)"
        fi
    fi
}

install_ansible() {
    highlight "处理 Ansible 工具"

    if is_installed ansible; then
        info "ansible 已安装: $(ansible --version | head -n1)"
        return
    fi

    read -p "是否安装 Ansible? (y/n): " choice
    if [ "$choice" = "y" ] || [ "$choice" = "Y" ]; then
        install_package "ansible"

        if is_installed ansible; then
            success "Ansible 安装完成: $(ansible --version | head -n1)"
        fi
    fi
}

install_istioctl() {
    highlight "处理 Istioctl 工具"

    if is_installed istioctl; then
        info "istioctl 已安装: $(istioctl version --short)"
        return
    fi

    read -p "是否安装 Istioctl? (y/n): " choice
    if [ "$choice" = "y" ] || [ "$choice" = "Y" ]; then
        # 安装Istioctl
        ISTIO_VERSION=$(curl -s https://api.github.com/repos/istio/istio/releases/latest | grep -oP '"tag_name": "\K(.*)(?=")')
        curl -L "https://istio.io/downloadIstioctl/$ISTIO_VERSION" | sh - > /dev/null 2>&1
        mv "$HOME/.istioctl/bin/istioctl" /usr/local/bin/

        if is_installed istioctl; then
            success "Istioctl 安装完成: $(istioctl version --short)"
        fi
    fi
}

install_kind() {
    highlight "处理 Kind 工具"

    if is_installed kind; then
        info "kind 已安装: $(kind version)"
        return
    fi

    read -p "是否安装 Kind? (y/n): " choice
    if [ "$choice" = "y" ] || [ "$choice" = "Y" ]; then
        # 安装Kind
        curl -Lo ./kind https://kind.sigs.k8s.io/dl/latest/kind-linux-amd64 > /dev/null 2>&1
        chmod +x ./kind
        mv ./kind /usr/local/bin/

        if is_installed kind; then
            success "Kind 安装完成: $(kind version)"
        fi
    fi
}

install_kubectx() {
    highlight "处理 kubectx/kubens 工具"

    if is_installed kubectx; then
        info "kubectx 已安装"
        return
    fi

    read -p "是否安装 kubectx 和 kubens? (y/n): " choice
    if [ "$choice" = "y" ] || [ "$choice" = "Y" ]; then
        # 安装kubectx和kubens
        git clone https://github.com/ahmetb/kubectx /opt/kubectx > /dev/null 2>&1
        ln -s /opt/kubectx/kubectx /usr/local/bin/kubectx
        ln -s /opt/kubectx/kubens /usr/local/bin/kubens

        if is_installed kubectx; then
            success "kubectx 和 kubens 安装完成"
        fi
    fi
}

# 云平台CLI工具安装函数（支持单独调用）
install_aws_cli() {
    highlight "处理 AWS CLI 工具"

    if is_installed aws; then
        info "AWS CLI 已安装: $(aws --version 2>&1 | head -n1)"
        return
    fi

    read -p "是否安装 AWS CLI? (y/n): " choice
    if [ "$choice" = "y" ] || [ "$choice" = "Y" ]; then
        # 安装AWS CLI
        curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip" > /dev/null 2>&1
        unzip -q awscliv2.zip
        ./aws/install > /dev/null 2>&1
        rm -rf aws awscliv2.zip

        if is_installed aws; then
            success "AWS CLI 安装完成: $(aws --version 2>&1 | head -n1)"
        fi
    fi
}

install_azure_cli() {
    highlight "处理 Azure CLI 工具"

    if is_installed az; then
        info "Azure CLI 已安装: $(az --version 2>&1 | head -n1)"
        return
    fi

    read -p "是否安装 Azure CLI? (y/n): " choice
    if [ "$choice" = "y" ] || [ "$choice" = "Y" ]; then
        # 安装Azure CLI
        curl -sL https://aka.ms/InstallAzureCLIDeb | bash > /dev/null 2>&1

        if is_installed az; then
            success "Azure CLI 安装完成: $(az --version 2>&1 | head -n1)"
        fi
    fi
}

install_gcloud() {
    highlight "处理 Google Cloud SDK 工具"

    if is_installed gcloud; then
        info "Google Cloud SDK 已安装"
        return
    fi

    read -p "是否安装 Google Cloud SDK? (y/n): " choice
    if [ "$choice" = "y" ] || [ "$choice" = "Y" ]; then
        # 安装Google Cloud SDK
        echo "deb [signed-by=/usr/share/keyrings/cloud.google.gpg] https://packages.cloud.google.com/apt cloud-sdk main" | tee -a /etc/apt/sources.list.d/google-cloud-sdk.list > /dev/null
        curl https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key --keyring /usr/share/keyrings/cloud.google.gpg add - > /dev/null 2>&1
        if [ "$OS_TYPE" = "debian" ]; then
            apt update -qq > /dev/null
            install_package "google-cloud-sdk"
        else
            curl https://sdk.cloud.google.com | bash -s -- --disable-prompts > /dev/null 2>&1
            echo "export PATH=\$HOME/google-cloud-sdk/bin:\$PATH" >> /etc/profile
            refresh_environment
        fi

        if is_installed gcloud; then
            success "Google Cloud SDK 安装完成"
        fi
    fi
}

# 常用开发工具安装函数（支持单独调用）
install_git() {
    highlight "处理 Git 工具"

    if is_installed git; then
        info "Git 已安装: $(git --version)"
        return
    fi

    read -p "是否安装 Git? (y/n): " choice
    if [ "$choice" = "y" ] || [ "$choice" = "Y" ]; then
        install_package "git"

        if is_installed git; then
            success "Git 安装完成: $(git --version)"
        fi
    fi
}

install_network_tools() {
    highlight "处理网络工具集"

    local tools=(
        "curl:HTTP客户端"
        "wget:文件下载工具"
        "net-tools:网络工具集"
        "iputils-ping:ping工具"
        "traceroute:路由跟踪工具"
    )

    for tool in "${tools[@]}"; do
        IFS=':' read -r cmd desc <<< "$tool"

        if is_installed "$cmd"; then
            info "$desc ($cmd) 已安装"
        else
            read -p "是否安装 $desc ($cmd)? (y/n): " choice
            if [ "$choice" = "y" ] || [ "$choice" = "Y" ]; then
                install_package "$cmd"
            fi
        fi
    done
}

install_editors() {
    highlight "处理文本编辑器"

    local tools=(
        "vim:文本编辑器"
        "nano:文本编辑器"
    )

    for tool in "${tools[@]}"; do
        IFS=':' read -r cmd desc <<< "$tool"

        if is_installed "$cmd"; then
            info "$desc ($cmd) 已安装"
        else
            read -p "是否安装 $desc ($cmd)? (y/n): " choice
            if [ "$choice" = "y" ] || [ "$choice" = "Y" ]; then
                install_package "$cmd"
            fi
        fi
    done
}

install_terminal_tools() {
    highlight "处理终端工具"

    local tools=(
        "tmux:终端复用工具"
        "zsh:Z shell"
    )

    for tool in "${tools[@]}"; do
        IFS=':' read -r cmd desc <<< "$tool"

        if is_installed "$cmd"; then
            info "$desc ($cmd) 已安装"
        else
            read -p "是否安装 $desc ($cmd)? (y/n): " choice
            if [ "$choice" = "y" ] || [ "$choice" = "Y" ]; then
                install_package "$cmd"
            fi
        fi
    done

    # 安装oh-my-zsh（如果已安装zsh）
    if is_installed zsh && [ ! -d "$HOME/.oh-my-zsh" ]; then
        read -p "是否安装 oh-my-zsh? (y/n): " choice
        if [ "$choice" = "y" ] || [ "$choice" = "Y" ]; then
            sh -c "$(curl -fsSL https://raw.githubusercontent.com/ohmyzsh/ohmyzsh/master/tools/install.sh)" "" --unattended > /dev/null 2>&1
            success "oh-my-zsh 安装完成"
        fi
    fi
}

install_system_tools() {
    highlight "处理系统工具"

    local tools=(
        "jq:JSON处理工具"
        "tree:目录树显示工具"
        "htop:系统监控工具"
        "ncdu:磁盘使用分析工具"
        "unzip:解压缩工具"
        "tar:归档工具"
        "gzip:压缩工具"
    )

    for tool in "${tools[@]}"; do
        IFS=':' read -r cmd desc <<< "$tool"

        if is_installed "$cmd"; then
            info "$desc ($cmd) 已安装"
        else
            read -p "是否安装 $desc ($cmd)? (y/n): " choice
            if [ "$choice" = "y" ] || [ "$choice" = "Y" ]; then
                install_package "$cmd"
            fi
        fi
    done
}

# 类别1：编程语言环境子菜单
programming_languages_menu() {
    while true; do
        display_header
        echo "===== 编程语言环境 ====="
        echo "1. Python 3 及 pip"
        echo "2. OpenJDK (Java)"
        echo "3. Node.js 及 npm/yarn"
        echo "4. Go 语言"
        echo "5. 安装本类别所有工具"
        echo "0. 返回主菜单"
        echo
        read -p "请输入选项 (0-5): " CHOICE

        case $CHOICE in
            1) install_python; press_any_key ;;
            2) install_java; press_any_key ;;
            3) install_nodejs; press_any_key ;;
            4) install_go; press_any_key ;;
            5)
                info "开始安装本类别所有工具..."
                # 非交互式一键安装所有
                install_python_non_interactive
                install_java_non_interactive
                install_nodejs_non_interactive
                install_go_non_interactive
                success "本类别所有工具安装完成"
                press_any_key
                ;;
            0) break ;;
            *) warning "无效选项，请输入0-5之间的数字"; press_any_key ;;
        esac
    done
}

# 非交互式安装函数（用于一键安装）
install_python_non_interactive() {
    highlight "处理 Python 环境 (非交互式)"

    if ! is_installed python3; then
        info "安装 Python3..."
        install_package "python3"
        install_package "python3-pip"
        ln -s /usr/bin/pip3 /usr/bin/pip 2>/dev/null
        success "Python3 环境配置完成"
    else
        info "Python3 已安装: $(python3 --version 2>&1)"
    fi
}

install_java_non_interactive() {
    highlight "处理 Java 环境 (非交互式)"

    if ! is_installed java; then
        info "安装 OpenJDK 11..."
        if [ "$OS_TYPE" = "debian" ]; then
            install_package "openjdk-11-jdk"
        else
            install_package "java-11-openjdk-devel"
        fi

        # 配置环境变量
        echo "export JAVA_HOME=$(dirname $(dirname $(readlink $(readlink $(which java)))))" >> /etc/profile
        echo "export PATH=\$JAVA_HOME/bin:\$PATH" >> /etc/profile
        refresh_environment

        success "OpenJDK 11 安装完成: $(java -version 2>&1 | head -n1)"
    else
        info "Java 已安装: $(java -version 2>&1 | head -n1)"
    fi
}

install_nodejs_non_interactive() {
    highlight "处理 Node.js 环境 (非交互式)"

    if ! is_installed node; then
        info "安装 Node.js 18..."
        curl -fsSL https://deb.nodesource.com/setup_18.x | bash - > /dev/null 2>&1
        install_package "nodejs"

        # 安装yarn
        info "安装 yarn..."
        npm install -g yarn > /dev/null 2>&1

        success "Node.js 18 安装完成: $(node --version)"
        success "npm 安装完成: $(npm --version)"
        success "yarn 安装完成: $(yarn --version)"
    else
        info "Node.js 已安装: $(node --version)"
        info "npm 已安装: $(npm --version)"
    fi
}

install_go_non_interactive() {
    highlight "处理 Go 环境 (非交互式)"

    if ! is_installed go; then
        local go_version="1.21.4"
        info "安装 Go $go_version..."

        # 下载并安装Go
        GO_TAR="go$go_version.linux-amd64.tar.gz"
        wget -q "https://dl.google.com/go/$GO_TAR" -O "/tmp/$GO_TAR"

        # 移除旧版本并安装新版本
        rm -rf /usr/local/go
        tar -C /usr/local -xzf "/tmp/$GO_TAR" > /dev/null
        rm "/tmp/$GO_TAR"

        # 配置环境变量
        echo "export GOROOT=/usr/local/go" >> /etc/profile
        echo "export GOPATH=\$HOME/go" >> /etc/profile
        echo "export PATH=\$GOROOT/bin:\$GOPATH/bin:\$PATH" >> /etc/profile
        refresh_environment

        # 创建GOPATH目录
        mkdir -p "$HOME/go"

        success "Go $go_version 安装完成: $(go version)"
    else
        info "Go 已安装: $(go version)"
    fi
}

# 类别2：容器与Kubernetes工具子菜单
container_k8s_menu() {
    while true; do
        display_header
        echo "===== 容器与Kubernetes工具 ====="
        echo "1. Docker 引擎"
        echo "2. kubectl (Kubernetes CLI)"
        echo "3. Helm (Kubernetes包管理器)"
        echo "4. Minikube (本地Kubernetes)"
        echo "5. 安装本类别所有工具"
        echo "0. 返回主菜单"
        echo
        read -p "请输入选项 (0-5): " CHOICE

        case $CHOICE in
            1) install_docker; press_any_key ;;
            2) install_kubectl; press_any_key ;;
            3) install_helm; press_any_key ;;
            4) install_minikube; press_any_key ;;
            5)
                info "开始安装本类别所有工具..."
                install_docker_non_interactive
                install_kubectl_non_interactive
                install_helm_non_interactive
                install_minikube_non_interactive
                success "本类别所有工具安装完成"
                press_any_key
                ;;
            0) break ;;
            *) warning "无效选项，请输入0-5之间的数字"; press_any_key ;;
        esac
    done
}

# 容器与K8s非交互式安装
install_docker_non_interactive() {
    highlight "处理 Docker 环境 (非交互式)"

    if ! is_installed docker; then
        info "安装 Docker..."
        # 安装依赖
        if [ "$OS_TYPE" = "debian" ]; then
            # 添加Docker官方GPG密钥
            curl -fsSL https://download.docker.com/linux/ubuntu/gpg | apt-key add - > /dev/null 2>&1

            # 添加Docker仓库
            add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" > /dev/null 2>&1
            apt update -qq > /dev/null
            install_package "docker-ce"
        else
            # RHEL/CentOS安装Docker
            yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo > /dev/null 2>&1
            install_package "docker-ce"
        fi

        # 启动并设置开机自启
        systemctl start docker
        systemctl enable docker > /dev/null

        # 将当前用户添加到docker组
        usermod -aG docker "$SUDO_USER"
        warning "用户已添加到docker组，需重新登录生效"

        success "Docker 安装完成: $(docker --version)"
    else
        info "Docker 已安装: $(docker --version)"
    fi
}

install_kubectl_non_interactive() {
    highlight "处理 kubectl 工具 (非交互式)"

    if ! is_installed kubectl; then
        info "安装 kubectl..."
        # 下载kubectl
        curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" > /dev/null 2>&1
        chmod +x kubectl
        mv kubectl /usr/local/bin/

        success "kubectl 安装完成: $(kubectl version --client --short)"
    else
        info "kubectl 已安装: $(kubectl version --client --short)"
    fi
}

install_helm_non_interactive() {
    highlight "处理 Helm 工具 (非交互式)"

    if ! is_installed helm; then
        info "安装 Helm..."
        # 下载Helm
        curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 > /dev/null 2>&1
        chmod 700 get_helm.sh
        ./get_helm.sh > /dev/null 2>&1
        rm get_helm.sh

        success "Helm 安装完成: $(helm version --short)"
    else
        info "Helm 已安装: $(helm version --short)"
    fi
}

install_minikube_non_interactive() {
    highlight "处理 Minikube 工具 (非交互式)"

    if ! is_installed minikube; then
        info "安装 Minikube..."
        # 下载Minikube
        curl -LO https://storage.googleapis.com/minikube/releases/latest/minikube-linux-amd64 > /dev/null 2>&1
        install -o root -g root -m 0755 minikube-linux-amd64 /usr/local/bin/minikube
        rm minikube-linux-amd64

        success "Minikube 安装完成: $(minikube version)"
    else
        info "Minikube 已安装: $(minikube version)"
    fi
}

# 类别3：云原生工具子菜单
cloud_native_menu() {
    while true; do
        display_header
        echo "===== 云原生工具 ====="
        echo "1. Terraform (基础设施即代码)"
        echo "2. Ansible (自动化运维工具)"
        echo "3. Istioctl (Istio服务网格)"
        echo "4. Kind (Kubernetes-in-Docker)"
        echo "5. kubectx/kubens (K8s上下文工具)"
        echo "6. 安装本类别所有工具"
        echo "0. 返回主菜单"
        echo
        read -p "请输入选项 (0-6): " CHOICE

        case $CHOICE in
            1) install_terraform; press_any_key ;;
            2) install_ansible; press_any_key ;;
            3) install_istioctl; press_any_key ;;
            4) install_kind; press_any_key ;;
            5) install_kubectx; press_any_key ;;
            6)
                info "开始安装本类别所有工具..."
                install_terraform_non_interactive
                install_ansible_non_interactive
                install_istioctl_non_interactive
                install_kind_non_interactive
                install_kubectx_non_interactive
                success "本类别所有工具安装完成"
                press_any_key
                ;;
            0) break ;;
            *) warning "无效选项，请输入0-6之间的数字"; press_any_key ;;
        esac
    done
}

# 云原生工具非交互式安装
install_terraform_non_interactive() {
    highlight "处理 Terraform 工具 (非交互式)"

    if ! is_installed terraform; then
        info "安装 Terraform..."
        if [ "$OS_TYPE" = "debian" ]; then
            curl -fsSL https://apt.releases.hashicorp.com/gpg | apt-key add - > /dev/null 2>&1
            apt-add-repository "deb [arch=amd64] https://apt.releases.hashicorp.com $(lsb_release -cs) main" > /dev/null 2>&1
            apt update -qq > /dev/null
        else
            yum install -qq -y yum-utils > /dev/null
            yum-config-manager --add-repo https://rpm.releases.hashicorp.com/RHEL/hashicorp.repo > /dev/null 2>&1
        fi
        install_package "terraform"

        success "Terraform 安装完成: $(terraform --version | head -n1)"
    else
        info "terraform 已安装: $(terraform --version | head -n1)"
    fi
}

install_ansible_non_interactive() {
    highlight "处理 Ansible 工具 (非交互式)"

    if ! is_installed ansible; then
        info "安装 Ansible..."
        install_package "ansible"

        success "Ansible 安装完成: $(ansible --version | head -n1)"
    else
        info "ansible 已安装: $(ansible --version | head -n1)"
    fi
}

install_istioctl_non_interactive() {
    highlight "处理 Istioctl 工具 (非交互式)"

    if ! is_installed istioctl; then
        info "安装 Istioctl..."
        # 安装Istioctl
        ISTIO_VERSION=$(curl -s https://api.github.com/repos/istio/istio/releases/latest | grep -oP '"tag_name": "\K(.*)(?=")')
        curl -L "https://istio.io/downloadIstioctl/$ISTIO_VERSION" | sh - > /dev/null 2>&1
        mv "$HOME/.istioctl/bin/istioctl" /usr/local/bin/

        success "Istioctl 安装完成: $(istioctl version --short)"
    else
        info "istioctl 已安装: $(istioctl version --short)"
    fi
}

install_kind_non_interactive() {
    highlight "处理 Kind 工具 (非交互式)"

    if ! is_installed kind; then
        info "安装 Kind..."
        # 安装Kind
        curl -Lo ./kind https://kind.sigs.k8s.io/dl/latest/kind-linux-amd64 > /dev/null 2>&1
        chmod +x ./kind
        mv ./kind /usr/local/bin/

        success "Kind 安装完成: $(kind version)"
    else
        info "kind 已安装: $(kind version)"
    fi
}

install_kubectx_non_interactive() {
    highlight "处理 kubectx/kubens 工具 (非交互式)"

    if ! is_installed kubectx; then
        info "安装 kubectx 和 kubens..."
        # 安装kubectx和kubens
        git clone https://github.com/ahmetb/kubectx /opt/kubectx > /dev/null 2>&1
        ln -s /opt/kubectx/kubectx /usr/local/bin/kubectx
        ln -s /opt/kubectx/kubens /usr/local/bin/kubens

        success "kubectx 和 kubens 安装完成"
    else
        info "kubectx 已安装"
    fi
}

# 类别4：云平台CLI工具子菜单
cloud_cli_menu() {
    while true; do
        display_header
        echo "===== 云平台CLI工具 ====="
        echo "1. AWS CLI (亚马逊云服务)"
        echo "2. Azure CLI (微软Azure)"
        echo "3. Google Cloud SDK (谷歌云平台)"
        echo "4. 安装本类别所有工具"
        echo "0. 返回主菜单"
        echo
        read -p "请输入选项 (0-4): " CHOICE

        case $CHOICE in
            1) install_aws_cli; press_any_key ;;
            2) install_azure_cli; press_any_key ;;
            3) install_gcloud; press_any_key ;;
            4)
                info "开始安装本类别所有工具..."
                install_aws_cli_non_interactive
                install_azure_cli_non_interactive
                install_gcloud_non_interactive
                success "本类别所有工具安装完成"
                press_any_key
                ;;
            0) break ;;
            *) warning "无效选项，请输入0-4之间的数字"; press_any_key ;;
        esac
    done
}

# 云平台CLI非交互式安装
install_aws_cli_non_interactive() {
    highlight "处理 AWS CLI 工具 (非交互式)"

    if ! is_installed aws; then
        info "安装 AWS CLI..."
        # 安装AWS CLI
        curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip" > /dev/null 2>&1
        unzip -q awscliv2.zip
        ./aws/install > /dev/null 2>&1
        rm -rf aws awscliv2.zip

        success "AWS CLI 安装完成: $(aws --version 2>&1 | head -n1)"
    else
        info "AWS CLI 已安装: $(aws --version 2>&1 | head -n1)"
    fi
}

install_azure_cli_non_interactive() {
    highlight "处理 Azure CLI 工具 (非交互式)"

    if ! is_installed az; then
        info "安装 Azure CLI..."
        # 安装Azure CLI
        curl -sL https://aka.ms/InstallAzureCLIDeb | bash > /dev/null 2>&1

        success "Azure CLI 安装完成: $(az --version 2>&1 | head -n1)"
    else
        info "Azure CLI 已安装: $(az --version 2>&1 | head -n1)"
    fi
}

install_gcloud_non_interactive() {
    highlight "处理 Google Cloud SDK 工具 (非交互式)"

    if ! is_installed gcloud; then
        info "安装 Google Cloud SDK..."
        # 安装Google Cloud SDK
        echo "deb [signed-by=/usr/share/keyrings/cloud.google.gpg] https://packages.cloud.google.com/apt cloud-sdk main" | tee -a /etc/apt/sources.list.d/google-cloud-sdk.list > /dev/null
        curl https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key --keyring /usr/share/keyrings/cloud.google.gpg add - > /dev/null 2>&1
        if [ "$OS_TYPE" = "debian" ]; then
            apt update -qq > /dev/null
            install_package "google-cloud-sdk"
        else
            curl https://sdk.cloud.google.com | bash -s -- --disable-prompts > /dev/null 2>&1
            echo "export PATH=\$HOME/google-cloud-sdk/bin:\$PATH" >> /etc/profile
            refresh_environment
        fi

        success "Google Cloud SDK 安装完成"
    else
        info "Google Cloud SDK 已安装"
    fi
}

# 类别5：常用开发工具子菜单
dev_tools_menu() {
    while true; do
        display_header
        echo "===== 常用开发工具 ====="
        echo "1. Git (版本控制)"
        echo "2. 网络工具 (curl, wget等)"
        echo "3. 文本编辑器 (vim, nano等)"
        echo "4. 终端工具 (tmux, zsh等)"
        echo "5. 系统工具 (htop, jq等)"
        echo "6. 安装本类别所有工具"
        echo "0. 返回主菜单"
        echo
        read -p "请输入选项 (0-6): " CHOICE

        case $CHOICE in
            1) install_git; press_any_key ;;
            2) install_network_tools; press_any_key ;;
            3) install_editors; press_any_key ;;
            4) install_terminal_tools; press_any_key ;;
            5) install_system_tools; press_any_key ;;
            6)
                info "开始安装本类别所有工具..."
                install_git_non_interactive
                install_network_tools_non_interactive
                install_editors_non_interactive
                install_terminal_tools_non_interactive
                install_system_tools_non_interactive
                success "本类别所有工具安装完成"
                press_any_key
                ;;
            0) break ;;
            *) warning "无效选项，请输入0-6之间的数字"; press_any_key ;;
        esac
    done
}

# 常用开发工具非交互式安装
install_git_non_interactive() {
    highlight "处理 Git 工具 (非交互式)"

    if ! is_installed git; then
        info "安装 Git..."
        install_package "git"

        success "Git 安装完成: $(git --version)"
    else
        info "Git 已安装: $(git --version)"
    fi
}

install_network_tools_non_interactive() {
    highlight "处理网络工具集 (非交互式)"

    local tools=(
        "curl"
        "wget"
        "net-tools"
        "iputils-ping"
        "traceroute"
    )

    for tool in "${tools[@]}"; do
        if ! is_installed "$tool"; then
            info "安装 $tool..."
            install_package "$tool"
        else
            info "$tool 已安装"
        fi
    done
}

install_editors_non_interactive() {
    highlight "处理文本编辑器 (非交互式)"

    local tools=(
        "vim"
        "nano"
    )

    for tool in "${tools[@]}"; do
        if ! is_installed "$tool"; then
            info "安装 $tool..."
            install_package "$tool"
        else
            info "$tool 已安装"
        fi
    done
}

install_terminal_tools_non_interactive() {
    highlight "处理终端工具 (非交互式)"

    local tools=(
        "tmux"
        "zsh"
    )

    for tool in "${tools[@]}"; do
        if ! is_installed "$tool"; then
            info "安装 $tool..."
            install_package "$tool"
        else
            info "$tool 已安装"
        fi
    done

    # 安装oh-my-zsh（如果已安装zsh）
    if is_installed zsh && [ ! -d "$HOME/.oh-my-zsh" ]; then
        info "安装 oh-my-zsh..."
        sh -c "$(curl -fsSL https://raw.githubusercontent.com/ohmyzsh/ohmyzsh/master/tools/install.sh)" "" --unattended > /dev/null 2>&1
        success "oh-my-zsh 安装完成"
    fi
}

install_system_tools_non_interactive() {
    highlight "处理系统工具 (非交互式)"

    local tools=(
        "jq"
        "tree"
        "htop"
        "ncdu"
        "unzip"
        "tar"
        "gzip"
    )

    for tool in "${tools[@]}"; do
        if ! is_installed "$tool"; then
            info "安装 $tool..."
            install_package "$tool"
        else
            info "$tool 已安装"
        fi
    done
}

# 主菜单
main_menu() {
    display_header

    while true; do
        echo "请选择要处理的组件类别:"
        echo "1. 编程语言环境 (Python, Java, Node.js, Go等)"
        echo "2. 容器与Kubernetes工具 (Docker, kubectl, Helm等)"
        echo "3. 云原生工具 (Terraform, Ansible, Istio等)"
        echo "4. 云平台CLI工具 (AWS, Azure, GCP等)"
        echo "5. 常用开发工具 (Git, Vim, Tmux等)"
        echo "6. 全部安装 (按类别依次进行)"
        echo "0. 退出脚本"
        echo
        read -p "请输入选项 (0-6): " CHOICE

        case $CHOICE in
            1) programming_languages_menu ;;
            2) container_k8s_menu ;;
            3) cloud_native_menu ;;
            4) cloud_cli_menu ;;
            5) dev_tools_menu ;;
            6)
                info "开始安装所有类别工具，这将需要较长时间..."
                programming_languages_menu_non_interactive
                container_k8s_menu_non_interactive
                cloud_native_menu_non_interactive
                cloud_cli_menu_non_interactive
                dev_tools_menu_non_interactive
                success "所有工具安装完成"
                press_any_key
                ;;
            0)
                display_header
                echo "系统环境配置完成！"
                echo "部分配置可能需要重新登录才能生效。"
                echo
                exit 0
                ;;
            *)
                warning "无效选项，请输入0-6之间的数字"
                press_any_key
                ;;
        esac
    done
}

# 全类别非交互式安装
programming_languages_menu_non_interactive() {
    highlight "===== 安装所有编程语言环境 ====="
    install_python_non_interactive
    install_java_non_interactive
    install_nodejs_non_interactive
    install_go_non_interactive
}

container_k8s_menu_non_interactive() {
    highlight "===== 安装所有容器与Kubernetes工具 ====="
    install_docker_non_interactive
    install_kubectl_non_interactive
    install_helm_non_interactive
    install_minikube_non_interactive
}

cloud_native_menu_non_interactive() {
    highlight "===== 安装所有云原生工具 ====="
    install_terraform_non_interactive
    install_ansible_non_interactive
    install_istioctl_non_interactive
    install_kind_non_interactive
    install_kubectx_non_interactive
}

cloud_cli_menu_non_interactive() {
    highlight "===== 安装所有云平台CLI工具 ====="
    install_aws_cli_non_interactive
    install_azure_cli_non_interactive
    install_gcloud_non_interactive
}

dev_tools_menu_non_interactive() {
    highlight "===== 安装所有常用开发工具 ====="
    install_git_non_interactive
    install_network_tools_non_interactive
    install_editors_non_interactive
    install_terminal_tools_non_interactive
    install_system_tools_non_interactive
}

# 清屏并显示标题
display_header() {
    clear
    echo "============================================="
    echo "          Linux系统开发环境配置工具           "
    echo "============================================="
    echo
}

# 等待用户按键继续
press_any_key() {
    echo
    read -n 1 -s -r -p "按任意键继续..."
}

# 主程序
display_header
detect_os
initialize_package_manager
main_menu