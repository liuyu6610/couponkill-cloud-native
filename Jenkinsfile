pipeline {
  agent any
  options { timestamps() }
  environment {
    REG = 'registry.example.com/couponkill'
  }
  stages {
    stage('Checkout'){ steps{ checkout scm } }
    stage('Build & Test'){
      steps {
        sh 'mvn -T1C -DskipTests=false test package'
      }
    }
    stage('Build Images'){
      steps {
        sh 'docker build -t $REG/coupon:$(git rev-parse --short HEAD) -f couponkill-coupon-service/Dockerfile .'
        sh 'docker build -t $REG/order:$(git rev-parse --short HEAD) -f couponkill-order-service/Dockerfile .'
      }
    }
    stage('Push'){ steps{ sh 'docker push $REG/coupon:$(git rev-parse --short HEAD); docker push $REG/order:$(git rev-parse --short HEAD)' } }
    stage('Deploy Dev'){
      steps {
        sh '''
        helm upgrade --install couponkill charts/               --namespace dev --create-namespace               --set image.coupon.tag=$(git rev-parse --short HEAD)               --set image.order.tag=$(git rev-parse --short HEAD)
        '''
      }
    }
    stage('Perf (JMeter)'){
      steps{
        sh '''
        docker run --rm -v $WORKSPACE/jmeter:/scripts ghcr.io/graalvm/jdk-community:latest bash -lc "echo JMeter placeholder"
        '''
      }
    }
  }
}
