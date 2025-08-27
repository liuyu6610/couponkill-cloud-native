// Jenkinsfile
pipeline {
    agent any
    
    environment {
        REGISTRY = "crpi-n5rumpjwbqinoz4c.cn-hangzhou.personal.cr.aliyuncs.com/thetestspacefordocker/my-docker"
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
                    }
                }
                stage('Coupon Service') {
                    steps {
                        sh 'docker build -t ${REGISTRY}/coupon:${BUILD_NUMBER} -f couponkill-coupon-service/Dockerfile .'
                    }
                }
                stage('Order Service') {
                    steps {
                        sh 'docker build -t ${REGISTRY}/order:${BUILD_NUMBER} -f couponkill-order-service/Dockerfile .'
                    }
                }
                stage('User Service') {
                    steps {
                        sh 'docker build -t ${REGISTRY}/user:${BUILD_NUMBER} -f couponkill-user-service/Dockerfile .'
                    }
                }
                stage('Go Service') {
                    steps {
                        sh 'docker build -t ${REGISTRY}/seckill-go:${BUILD_NUMBER} -f couponkill-go-service/Dockerfile .'
                    }
                }
                stage('Operator') {
                    steps {
                        sh 'docker build -t ${REGISTRY}/operator:${BUILD_NUMBER} -f couponkill-operator/Dockerfile .'
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
                    sh 'docker push ${REGISTRY}/seckill-go:${BUILD_NUMBER}'
                    sh 'docker push ${REGISTRY}/operator:${BUILD_NUMBER}'
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