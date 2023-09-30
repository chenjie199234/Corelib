package deploy

import (
	"os"
	"text/template"
)

const docker = `FROM golang:1.18-buster as builder
ENV GOSUMDB='off' \
	GOOS='linux' \
	GOARCH='amd64' \
	CGO_ENABLED=0

RUN mkdir /code
ADD . /code
WORKDIR /code
RUN echo "start build" && go mod tidy && go build -o main && echo "end build"

FROM debian:buster
RUN apt-get update && apt-get install -y ca-certificates curl inetutils-telnet inetutils-ping inetutils-traceroute dnsutils iproute2 procps net-tools neovim && mkdir /root/app
WORKDIR /root/app
EXPOSE 6060 8000 9000 10000
COPY --from=builder /code/main /code/AppConfig.json /code/SourceConfig.json ./
ENTRYPOINT ["./main"]`

const deployment = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{.AppName}}-deployment
  namespace: <PROJECT>-<GROUP>
  labels:
    app: {{.AppName}}
spec:
  replicas: 2
  minReadySeconds: 5
  progressDeadlineSeconds: 300
  revisionHistoryLimit: 5
  selector:
    matchLabels:
      app: {{.AppName}}
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0
  template:
    metadata:
      labels:
        app: {{.AppName}}
    spec:
      containers:
        - name: {{.AppName}}
          image: <IMAGE>
          imagePullPolicy: IfNotPresent
          ports:
            - name: web
              protocol: TCP
              containerPort: 8000
            - name: crpc
              protocol: TCP
              containerPort: 9000
            - name: grpc
              protocol: TCP
              containerPort: 10000
          resources:
            limits:
              memory: 4096Mi
              cpu: 4000m
            requests:
              memory: 256Mi
              cpu: 250m
          env:
            - name: HOSTIP
              valueFrom:
                fieldRef:
                  fieldPath: status.podIP
            - name: GROUP
              value: <GROUP>
            - name: PROJECT
              value: <PROJECT>
            - name: LOG_LEVEL
              value: <LOG_LEVEL>
            - name: LOG_TRACE
              value: <LOG_TRACE>
            - name: LOG_TARGET
              value: <LOG_TARGET>
            - name: DEPLOY_ENV
              value: <DEPLOY_ENV>
            - name: RUN_ENV
              value: <RUN_ENV>
            - name: MONITOR
              value: <MONITOR>
            - name: CONFIG_TYPE
              value: <CONFIG_TYPE>
            - name: REMOTE_CONFIG_SECRET
              value: <REMOTE_CONFIG_SECRET>
            - name: ADMIN_SERVICE_PROJECT
              value: <ADMIN_SERVICE_PROJECT>
            - name: ADMIN_SERVICE_GROUP
              value: <ADMIN_SERVICE_GROUP>
            - name: ADMIN_SERVICE_WEB_HOST
              value: <ADMIN_SERVICE_WEB_HOST>
            - name: ADMIN_SERVICE_WEB_PORT
              value: <ADMIN_SERVICE_WEB_PORT>
          startupProbe:
            tcpSocket:
              port: 8000
            initialDelaySeconds: 5
            timeoutSeconds: 1
            periodSeconds: 1
            successThreshold: 1
            failureThreshold: 3
          livenessProbe:
            tcpSocket:
              port: 8000
            initialDelaySeconds: 5
            timeoutSeconds: 1
            periodSeconds: 1
            successThreshold: 1
            failureThreshold: 3
      imagePullSecrets:
        - name: <PROJECT>-<GROUP>-secret
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: {{.AppName}}-hpa
  namespace: <PROJECT>-<GROUP>
  labels:
    app: {{.AppName}}
spec:
  scaleTargetRef:   
    apiVersion: apps/v1
    kind: Deployment  
    name: {{.AppName}}-deployment
  maxReplicas: 10
  minReplicas: 2
  metrics:
    - type: Resource
      resource:
        name: memory
        target:
          type: AverageValue
          averageValue: 3500Mi
    - type: Resource
      resource:
        name: cpu
        target:
          type: AverageValue
          averageValue: 3400m{{ if .NeedService }}
---
apiVersion: v1
kind: Service
metadata:
  name: {{.AppName}}-headless
  namespace: <PROJECT>-<GROUP>
  labels:
    app: {{.AppName}}
spec:
  type: ClusterIP
  clusterIP: None
  ports:
    - name: crpc
      protocol: TCP
      port: 9000
    - name: grpc
      protocol: TCP
      port: 10000
  selector:
    app: {{.AppName}}
---
apiVersion: v1
kind: Service
metadata:
  name: {{.AppName}}
  namespace: <PROJECT>-<GROUP>
  labels:
    app: {{.AppName}}
spec:
  type: ClusterIP
  ports:
    - name: web
      protocol: TCP
      port: 8000
  selector:
    app: {{.AppName}}{{ end }}{{ if .NeedIngress}}
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: {{.AppName}}-ingress
  namespace: <PROJECT>-<GROUP>
  labels:
    app: {{.AppName}}
  annotations:
    nginx.ingress.kubernetes.io/use-regex: 'true'
spec:
  rules: 
    - host: <HOST>
      http:
        paths:
          - path: /{{.AppName}}.*
            pathType: Prefix
            backend:
              service:
                name: {{.AppName}}
                port:
                  number: 8000{{ end }}`

type data struct {
	AppName     string
	NeedService bool
	NeedIngress bool
}

func CreatePathAndFile(appname string, needservice bool, needingress bool) {
	dockerfile, e := os.OpenFile("./Dockerfile", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./Dockerfile error: " + e.Error())
	}
	if _, e := dockerfile.WriteString(docker); e != nil {
		panic("write ./Dockerfile error: " + e.Error())
	}
	if e := dockerfile.Sync(); e != nil {
		panic("sync ./Dockerfile error: " + e.Error())
	}
	if e := dockerfile.Close(); e != nil {
		panic("close ./Dockerfile error: " + e.Error())
	}

	tmp := &data{
		AppName:     appname,
		NeedService: needservice,
		NeedIngress: needingress,
	}
	deploymenttemplate, e := template.New("./deployment.yaml").Parse(deployment)
	if e != nil {
		panic("parse ./deployment.yaml template error: " + e.Error())
	}
	kubernetesfile, e := os.OpenFile("./deployment.yaml", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./deployment.yaml error: " + e.Error())
	}
	if e := deploymenttemplate.Execute(kubernetesfile, tmp); e != nil {
		panic("write ./deployment.yaml error: " + e.Error())
	}
	if e := kubernetesfile.Sync(); e != nil {
		panic("sync ./deployment.yaml error: " + e.Error())
	}
	if e := kubernetesfile.Close(); e != nil {
		panic("close ./deployment.yaml error: " + e.Error())
	}
}
