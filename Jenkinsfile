// Jenkinsfile
pipeline {
    agent any

    environment {
        DOCKER_REGISTRY = "your-registry.example.com"
        PROJECT_NAME = "couponkill"
        VERSION = "1.0-${BUILD_NUMBER}"
    }

    stages {
        stage('Build') {
            steps {
                sh 'mvn clean package -Dmaven.test.skip=true'
            }
        }

        stage('Test') {
            steps {
                sh 'mvn test'
            }
            post {
                always {
                    junit '**/target/surefire-reports/*.xml'
                }
            }
        }

        stage('Build Docker Images') {
            parallel {
                stage('User Service') {
                    steps {
                        dir('couponkill-user-service') {
                            sh "docker build -t ${DOCKER_REGISTRY}/${PROJECT_NAME}/user-service:${VERSION} ."
                        }
                    }
                }
                stage('Coupon Service') {
                    steps {
                        dir('couponkill-coupon-service') {
                            sh "docker build -t ${DOCKER_REGISTRY}/${PROJECT_NAME}/coupon-service:${VERSION} ."
                        }
                    }
                }
                stage('Order Service') {
                    steps {
                        dir('couponkill-order-service') {
                            sh "docker build -t ${DOCKER_REGISTRY}/${PROJECT_NAME}/order-service:${VERSION} ."
                        }
                    }
                }
                stage('Admin Service') {
                    steps {
                        dir('couponkill-admin-service') {
                            sh "docker build -t ${DOCKER_REGISTRY}/${PROJECT_NAME}/admin-service:${VERSION} ."
                        }
                    }
                }
                stage('Gateway Service') {
                    steps {
                        dir('couponkill-gateway') {
                            sh "docker build -t ${DOCKER_REGISTRY}/${PROJECT_NAME}/gateway:${VERSION} ."
                        }
                    }
                }
                stage('Go Service') {
                    steps {
                        dir('couponkill-go-service') {
                            sh "docker build -t ${DOCKER_REGISTRY}/${PROJECT_NAME}/go-service:${VERSION} ."
                        }
                    }
                }
            }
        }

        stage('Push Docker Images') {
            steps {
                sh "docker login -u ${DOCKER_USERNAME} -p ${DOCKER_PASSWORD} ${DOCKER_REGISTRY}"
                sh "docker push ${DOCKER_REGISTRY}/${PROJECT_NAME}/user-service:${VERSION}"
                sh "docker push ${DOCKER_REGISTRY}/${PROJECT_NAME}/coupon-service:${VERSION}"
                sh "docker push ${DOCKER_REGISTRY}/${PROJECT_NAME}/order-service:${VERSION}"
                sh "docker push ${DOCKER_REGISTRY}/${PROJECT_NAME}/admin-service:${VERSION}"
                sh "docker push ${DOCKER_REGISTRY}/${PROJECT_NAME}/gateway:${VERSION}"
                sh "docker push ${DOCKER_REGISTRY}/${PROJECT_NAME}/go-service:${VERSION}"
            }
        }

        stage('Deploy to Kubernetes') {
            steps {
                sh "sed -i 's|IMAGE_TAG|${VERSION}|g' k8s/deployment.yaml"
                sh "kubectl apply -f k8s/deployment.yaml"
                sh "kubectl apply -f k8s/service.yaml"
                sh "kubectl apply -f k8s/ingress.yaml"
            }
        }
    }

    post {
        success {
            slackSend channel: '#deployments', color: 'good', message: "Successfully deployed ${PROJECT_NAME} version ${VERSION}"
        }
        failure {
            slackSend channel: '#deployments', color: 'danger', message: "Failed to deploy ${PROJECT_NAME} version ${VERSION}"
        }
    }
}