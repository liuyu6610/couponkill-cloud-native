// Jenkinsfile — 生产/演示集群 CD 真源（见 docs/CICD-SOURCE-OF-TRUTH.md）
// 部署制品以 charts/couponkill 为准（见 docs/DEPLOYMENT-SOURCE-OF-TRUTH.md）
pipeline {
    agent any
    
    environment {
        REGISTRY = "crpi-n5rumpjwbqinoz4c.cn-hangzhou.personal.cr.aliyuncs.com/thetestspacefordocker/my-docker"
        CANARY_REGISTRY = "crpi-n5rumpjwbqinoz4c.cn-hangzhou.personal.cr.aliyuncs.com/thetestspacefordocker/canary-keda-dev"
        NAMESPACE = "couponkill"
        HELM_CHART_PATH = "./charts/couponkill"
    }
    
    stages {
        stage('Build') {
            parallel {
                stage('Build Java Services') {
                    steps {
                        sh 'mvn clean package -DskipTests'
                    }
                }
                stage('Build Go Service') {
                    steps {
                        sh 'cd couponkill-go-service && go build -o seckill-go ./cmd/server'
                    }
                }
                stage('Build Operator') {
                    steps {
                        sh 'cd couponkill-operator && make generate && make manifests'
                    }
                }
            }
        }
        
        stage('Build Docker Images') {
            parallel {
                stage('Gateway Service') {
                    steps {
                        sh 'docker build -t ${REGISTRY}/gateway:${BUILD_NUMBER} -f couponkill-gateway/Dockerfile .'
                        sh 'docker build -t ${CANARY_REGISTRY}/gateway:canary -f couponkill-gateway/Dockerfile .'
                    }
                }
                stage('Coupon Service') {
                    steps {
                        sh 'docker build -t ${REGISTRY}/coupon:${BUILD_NUMBER} -f couponkill-coupon-service/Dockerfile .'
                        sh 'docker build -t ${CANARY_REGISTRY}/coupon:canary -f couponkill-coupon-service/Dockerfile .'
                    }
                }
                stage('Order Service') {
                    steps {
                        sh 'docker build -t ${REGISTRY}/order:${BUILD_NUMBER} -f couponkill-order-service/Dockerfile .'
                        sh 'docker build -t ${CANARY_REGISTRY}/order:canary -f couponkill-order-service/Dockerfile .'
                    }
                }
                stage('User Service') {
                    steps {
                        sh 'docker build -t ${REGISTRY}/user:${BUILD_NUMBER} -f couponkill-user-service/Dockerfile .'
                        sh 'docker build -t ${CANARY_REGISTRY}/user:canary -f couponkill-user-service/Dockerfile .'
                    }
                }
                stage('Connector Service') {
                    steps {
                        sh 'docker build -t ${REGISTRY}/connector:${BUILD_NUMBER} -f couponkill-connector-service/Dockerfile .'
                        sh 'docker build -t ${CANARY_REGISTRY}/connector:canary -f couponkill-connector-service/Dockerfile .'
                    }
                }
                stage('Go Service') {
                    steps {
                        sh 'docker build -t ${REGISTRY}/seckill-go:${BUILD_NUMBER} -f couponkill-go-service/Dockerfile .'
                        sh 'docker build -t ${CANARY_REGISTRY}/seckill-go:canary -f couponkill-go-service/Dockerfile .'
                    }
                }
                stage('Operator') {
                    steps {
                        sh 'docker build -t ${REGISTRY}/operator:${BUILD_NUMBER} -f couponkill-operator/Dockerfile .'
                        sh 'docker build -t ${CANARY_REGISTRY}/operator:canary -f couponkill-operator/Dockerfile .'
                    }
                }
            }
        }
        
        stage('Push Docker Images') {
            steps {
                withCredentials([usernamePassword(credentialsId: 'docker-registry', passwordVariable: 'DOCKER_PASSWORD', usernameVariable: 'DOCKER_USERNAME')]) {
                    sh 'echo ${DOCKER_PASSWORD} | docker login ${REGISTRY} -u ${DOCKER_USERNAME} --password-stdin'
                    sh 'docker push ${REGISTRY}/gateway:${BUILD_NUMBER}'
                    sh 'docker push ${REGISTRY}/coupon:${BUILD_NUMBER}'
                    sh 'docker push ${REGISTRY}/order:${BUILD_NUMBER}'
                    sh 'docker push ${REGISTRY}/user:${BUILD_NUMBER}'
                    sh 'docker push ${REGISTRY}/connector:${BUILD_NUMBER}'
                    sh 'docker push ${REGISTRY}/seckill-go:${BUILD_NUMBER}'
                    sh 'docker push ${REGISTRY}/operator:${BUILD_NUMBER}'
                    
                    // 推送金丝雀版本镜像
                    sh 'docker push ${CANARY_REGISTRY}/gateway:canary'
                    sh 'docker push ${CANARY_REGISTRY}/coupon:canary'
                    sh 'docker push ${CANARY_REGISTRY}/order:canary'
                    sh 'docker push ${CANARY_REGISTRY}/user:canary'
                    sh 'docker push ${CANARY_REGISTRY}/connector:canary'
                    sh 'docker push ${CANARY_REGISTRY}/seckill-go:canary'
                    sh 'docker push ${CANARY_REGISTRY}/operator:canary'
                }
            }
        }
        
        stage('Pull and Push Dependency Images') {
            steps {
                withCredentials([usernamePassword(credentialsId: 'docker-registry', passwordVariable: 'DOCKER_PASSWORD', usernameVariable: 'DOCKER_USERNAME')]) {
                    sh 'echo ${DOCKER_PASSWORD} | docker login ${REGISTRY} -u ${DOCKER_USERNAME} --password-stdin'
                    
                    // PostgreSQL（替代 MySQL）
                    sh 'docker pull postgres:16'
                    sh 'docker tag postgres:16 ${REGISTRY}/postgres'
                    sh 'docker push ${REGISTRY}/postgres'
                    
                    // Redis
                    sh 'docker pull redis:7.0'
                    sh 'docker tag redis:7.0 ${REGISTRY}/redis'
                    sh 'docker push ${REGISTRY}/redis'
                    
                    // Nacos
                    sh 'docker pull nacos/nacos-server:v3.1.1'
                    sh 'docker tag nacos/nacos-server:v3.1.1 ${REGISTRY}/nacos-server'
                    sh 'docker push ${REGISTRY}/nacos-server'
                    
                    // Sentinel
                    sh 'docker pull bladex/sentinel-dashboard:1.8.6'
                    sh 'docker tag bladex/sentinel-dashboard:1.8.6 ${REGISTRY}/sentinel-dashboard'
                    sh 'docker push ${REGISTRY}/sentinel-dashboard'
                    
                    // Kafka KRaft（不再拉取 RocketMQ / ZooKeeper）
                    sh 'docker pull apache/kafka:3.8.0'
                    sh 'docker tag apache/kafka:3.8.0 ${REGISTRY}/kafka'
                    sh 'docker push ${REGISTRY}/kafka'
                }
            }
        }
        
        stage('Deploy to Kubernetes') {
            steps {
                withCredentials([file(credentialsId: 'kubeconfig', variable: 'KUBECONFIG')]) {
                    sh 'helm upgrade --install couponkill ${HELM_CHART_PATH} --namespace ${NAMESPACE} --create-namespace --set image.tag=${BUILD_NUMBER}'
                }
            }
        }
        
        stage('Deploy Canary Release') {
            steps {
                withCredentials([file(credentialsId: 'kubeconfig', variable: 'KUBECONFIG')]) {
                    sh 'helm upgrade --install couponkill-canary ${HELM_CHART_PATH} --namespace ${NAMESPACE} --create-namespace -f ${HELM_CHART_PATH}/values.canary-keda.yaml'
                }
            }
        }
    }
    
    post {
        always {
            sh 'docker logout ${REGISTRY}'
        }
        success {
            slackSend channel: '#deployments', color: 'good', message: "CouponKill deployed successfully: Build #${BUILD_NUMBER}"
        }
        failure {
            slackSend channel: '#deployments', color: 'danger', message: "CouponKill deployment failed: Build #${BUILD_NUMBER}"
        }
    }
}