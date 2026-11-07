// Jenkinsfile — 生产/演示集群 CD 真源（见 docs/CICD-SOURCE-OF-TRUTH.md）
// 部署制品以 charts/couponkill 为准（见 docs/DEPLOYMENT-SOURCE-OF-TRUTH.md）
//
// 流程：test → 构建 5 个服务镜像（+ Operator）→ 推送 → helm upgrade。
// 镜像命名：<REGISTRY>/couponkill-<svc>:<BUILD_NUMBER>（与 build/Dockerfile、Makefile、charts/couponkill 一致）。
// 没有 Java / Maven / Nacos / Sentinel 步骤：配置全部是环境变量，迁移由服务启动时自己执行。
//
// 构建参数：
//   DEPLOY_MODE  stable  全量发布本次构建（默认）
//                canary  稳定版保持 STABLE_TAG，coupon / order 起金丝雀 Pod 跑本次构建
//                none    只构建并推送镜像，不部署
//   STABLE_TAG   canary 模式下稳定版使用的镜像标签（必填）
//
// Agent 需要：docker、helm、kubectl 可用；凭证 docker-registry（用户名口令）、kubeconfig（文件）。
pipeline {
    agent any

    options {
        timestamps()
        disableConcurrentBuilds()
        timeout(time: 60, unit: 'MINUTES')
    }

    parameters {
        choice(name: 'DEPLOY_MODE', choices: ['stable', 'canary', 'none'], description: '发布方式')
        string(name: 'STABLE_TAG', defaultValue: '', description: 'DEPLOY_MODE=canary 时稳定版使用的镜像标签')
    }

    environment {
        REGISTRY = "crpi-n5rumpjwbqinoz4c.cn-hangzhou.personal.cr.aliyuncs.com/thetestspacefordocker"
        NAMESPACE = "couponkill"
        RELEASE = "couponkill"
        HELM_CHART_PATH = "./charts/couponkill"
        // 与 go.mod 的 go 1.27.0 一致；GOTOOLCHAIN=local 禁止构建时下载别的工具链
        GO_IMAGE = "golang:1.27-alpine"
        PG_IMAGE = "postgres:16"
    }

    stages {
        stage('Test') {
            parallel {
                stage('Go services') {
                    steps {
                        // 在容器里编译/测试，不依赖 agent 自带的 Go；PostgreSQL 起一个临时容器，
                        // CK_REQUIRE_PG=true 让连不上 PG 的测试失败而不是跳过。
                        // -race 需要 cgo，由 GitHub Actions 的 CI 负责；这里 CD 只做 vet + test。
                        sh '''
                            set -eu
                            NET="ck-test-${BUILD_NUMBER}"
                            PG="${NET}-pg"
                            cleanup() { docker rm -f "$PG" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
                            trap cleanup EXIT
                            docker network create "$NET"
                            docker run -d --name "$PG" --network "$NET" -e POSTGRES_PASSWORD=postgres "$PG_IMAGE" >/dev/null
                            i=0
                            until docker exec "$PG" pg_isready -U postgres >/dev/null 2>&1; do
                                i=$((i + 1)); [ "$i" -lt 60 ] || { echo "PostgreSQL 未就绪"; exit 1; }
                                sleep 1
                            done
                            docker run --rm --network "$NET" -u "$(id -u):$(id -g)" \
                                -e HOME=/tmp -e GOCACHE=/tmp/go-build -e GOPATH=/tmp/go -e GOTOOLCHAIN=local -e CGO_ENABLED=0 \
                                -e CK_TEST_PG_DSN="postgres://postgres:postgres@${PG}:5432/postgres?sslmode=disable" \
                                -e CK_REQUIRE_PG=true \
                                -v "$WORKSPACE:/src" -w /src "$GO_IMAGE" \
                                sh -c "go vet ./cmd/... ./internal/... && go test -count=1 ./cmd/... ./internal/..."
                        '''
                    }
                }
                stage('Operator') {
                    steps {
                        sh '''
                            docker run --rm -u "$(id -u):$(id -g)" \
                                -e HOME=/tmp -e GOCACHE=/tmp/go-build -e GOPATH=/tmp/go -e GOTOOLCHAIN=local -e CGO_ENABLED=0 \
                                -v "$WORKSPACE/couponkill-operator:/src" -w /src "$GO_IMAGE" \
                                sh -c "go vet ./... && go test -count=1 ./..."
                        '''
                    }
                }
                stage('Helm chart') {
                    steps {
                        sh '''
                            helm lint "$HELM_CHART_PATH"
                            helm lint "$HELM_CHART_PATH" -f "$HELM_CHART_PATH/values-prod.yaml" --set-string image.tag="$BUILD_NUMBER"
                            helm lint "$HELM_CHART_PATH" -f "$HELM_CHART_PATH/values.canary-keda.yaml"
                        '''
                    }
                }
            }
        }

        stage('Build Docker Images') {
            steps {
                script {
                    // 服务 → 容器端口（EXPOSE 元数据；与设计文档 D3 一致）
                    def services = [gateway: '8088', user: '8081', coupon: '8080', order: '8082', connector: '8085']
                    def builds = services.collectEntries { svc, port ->
                        [(svc): {
                            sh """docker build -f build/Dockerfile \
                                    --build-arg SERVICE=${svc} --build-arg PORT=${port} --build-arg VCS_REF=${env.GIT_COMMIT ?: 'unknown'} \
                                    -t ${env.REGISTRY}/couponkill-${svc}:${env.BUILD_NUMBER} ."""
                        }]
                    }
                    builds['operator'] = {
                        sh "docker build -f couponkill-operator/Dockerfile -t ${env.REGISTRY}/couponkill-operator:${env.BUILD_NUMBER} ."
                    }
                    parallel builds
                }
            }
        }

        stage('Push Docker Images') {
            steps {
                withCredentials([usernamePassword(credentialsId: 'docker-registry', passwordVariable: 'DOCKER_PASSWORD', usernameVariable: 'DOCKER_USERNAME')]) {
                    sh 'echo "${DOCKER_PASSWORD}" | docker login "${REGISTRY}" -u "${DOCKER_USERNAME}" --password-stdin'
                    script {
                        ['gateway', 'user', 'coupon', 'order', 'connector', 'operator'].each { svc ->
                            sh "docker push ${env.REGISTRY}/couponkill-${svc}:${env.BUILD_NUMBER}"
                        }
                    }
                }
            }
        }

        // 生产必须预先创建 Secret couponkill-app-secrets（见 charts/couponkill/README.md）；
        // values-prod.yaml 在 config.env=prod 下拒绝 chart 生成密钥与 latest 标签。
        stage('Deploy to Kubernetes') {
            when { expression { params.DEPLOY_MODE == 'stable' } }
            steps {
                withCredentials([file(credentialsId: 'kubeconfig', variable: 'KUBECONFIG')]) {
                    sh '''
                        helm upgrade --install "$RELEASE" "$HELM_CHART_PATH" \
                            --namespace "$NAMESPACE" --create-namespace \
                            -f "$HELM_CHART_PATH/values-prod.yaml" \
                            --set-string image.tag="$BUILD_NUMBER" \
                            --atomic --wait --timeout 10m
                    '''
                }
            }
        }

        // 金丝雀：稳定版保持 STABLE_TAG，coupon / order 额外起 version=canary 的 Pod 跑本次构建，
        // Istio 按 values.canary-keda.yaml 的权重切流；KEDA 同时接管 order 的副本数。
        stage('Deploy Canary Release') {
            when { expression { params.DEPLOY_MODE == 'canary' } }
            steps {
                script {
                    if (!params.STABLE_TAG?.trim()) {
                        error('DEPLOY_MODE=canary 需要填写 STABLE_TAG（稳定版镜像标签）')
                    }
                }
                withCredentials([file(credentialsId: 'kubeconfig', variable: 'KUBECONFIG')]) {
                    sh '''
                        helm upgrade --install "$RELEASE" "$HELM_CHART_PATH" \
                            --namespace "$NAMESPACE" --create-namespace \
                            -f "$HELM_CHART_PATH/values-prod.yaml" \
                            -f "$HELM_CHART_PATH/values.canary-keda.yaml" \
                            --set-string image.tag="$STABLE_TAG" \
                            --set-string canary.coupon.tag="$BUILD_NUMBER" \
                            --set-string canary.order.tag="$BUILD_NUMBER" \
                            --atomic --wait --timeout 10m
                    '''
                }
            }
        }
    }

    post {
        always {
            sh 'docker logout "${REGISTRY}" || true'
        }
        success {
            slackSend channel: '#deployments', color: 'good', message: "CouponKill ${params.DEPLOY_MODE}: Build #${BUILD_NUMBER} succeeded"
        }
        failure {
            slackSend channel: '#deployments', color: 'danger', message: "CouponKill ${params.DEPLOY_MODE}: Build #${BUILD_NUMBER} failed"
        }
    }
}
