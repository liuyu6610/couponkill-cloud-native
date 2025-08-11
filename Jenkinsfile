pipeline {
  agent {
    kubernetes {
      label 'jenkins-couponkill'
      yaml """
apiVersion: v1
kind: Pod
spec:
  containers:
  - name: maven
    image: maven:3.9.4-eclipse-temurin-21
    command:
    - cat
    tty: true
  - name: golang
    image: golang:1.23
    command:
    - cat
    tty: true
"""
    }
  }
  environment {
    REGISTRY = "registry.example.com/couponkill"
    BRANCH_NAME = "${env.BRANCH_NAME}"
  }
  stages {
    stage('Build Java') {
      steps {
        container('maven') {
          sh 'mvn clean package -DskipTests'
        }
      }
    }
    stage('Build Go') {
      steps {
        container('golang') {
          sh 'cd couponkill-seckill-service && go build -o app main.go'
        }
      }
    }
    stage('Docker Build & Push') {
      steps {
        sh '''
        docker build -t $REGISTRY/java-service:$BRANCH_NAME .
        docker build -t $REGISTRY/go-seckill:$BRANCH_NAME ./couponkill-seckill-service
        docker push $REGISTRY/java-service:$BRANCH_NAME
        docker push $REGISTRY/go-seckill:$BRANCH_NAME
        '''
      }
    }
    stage('Deploy to K8s') {
      steps {
        sh 'kubectl apply -f deploy/k8s/'
      }
    }
  }
}
