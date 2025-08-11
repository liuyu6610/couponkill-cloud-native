pipeline {
  agent any
  environment {
    DOCKER_REGISTRY = credentials('DOCKER_REGISTRY') // Jenkins credential (username:password) or registry token
    IMAGE_TAG = "${env.BUILD_NUMBER ?: 'local'}"
    KUBE_CONFIG_CREDENTIAL_ID = 'kubeconfig-credentials' // place your kubeconfig in Jenkins credentials
    DOCKER_BUILDX = '' // optional
  }
  stages {
    stage('Checkout') {
      steps {
        checkout scm
      }
    }
    stage('Build') {
      steps {
        sh 'mvn -B -DskipTests clean package'
      }
    }
    stage('Unit Test') {
      steps {
        sh 'mvn test -q'
      }
    }
    stage('Build Docker Images') {
      steps {
        script {
          // Example: build user, order, seckill-go images if directories present
          sh '''
            docker build -t ${DOCKER_REGISTRY}/couponkill-user-service:${IMAGE_TAG} ./couponkill-user-service || true
            docker build -t ${DOCKER_REGISTRY}/couponkill-order-service:${IMAGE_TAG} ./couponkill-order-service || true
            docker build -t ${DOCKER_REGISTRY}/seckill-go:${IMAGE_TAG} ./seckill-go || true
          '''
        }
      }
    }
    stage('Push Images') {
      steps {
        script {
          sh '''
            docker login -u $DOCKER_REGISTRY_USR -p $DOCKER_REGISTRY_PSW ${DOCKER_REGISTRY_HOST}
            docker push ${DOCKER_REGISTRY}/couponkill-user-service:${IMAGE_TAG} || true
            docker push ${DOCKER_REGISTRY}/couponkill-order-service:${IMAGE_TAG} || true
            docker push ${DOCKER_REGISTRY}/seckill-go:${IMAGE_TAG} || true
          '''
        }
      }
    }
    stage('Deploy to Kubernetes') {
      steps {
        withCredentials([file(credentialsId: "${KUBE_CONFIG_CREDENTIAL_ID}", variable: 'KUBECONFIG_FILE')]) {
          sh '''
            export KUBECONFIG=${KUBECONFIG_FILE}
            kubectl apply -f k8s/ || true
          '''
        }
      }
    }
  }
  post {
    success {
      echo 'Pipeline succeeded'
    }
    failure {
      echo 'Pipeline failed'
    }
  }
}