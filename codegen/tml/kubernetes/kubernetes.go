package kubernetes

import (
	"fmt"
	"os"
	"text/template"
)

const dockerfiletext = `FROM busybox:1.28

RUN mkdir /root/app

COPY main AppConfig.json SourceConfig.json /root/app

WORKDIR /root/app

SHELL ["sh","-c"]

ENTRYPOINT ["./main"]`

const deploymenttext = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{.ProjectName}}-deployment
  namespace: {{.NameSpace}}
  labels:
    app: {{.ProjectName}}
spec:
  replicas: 1
  revisionHistoryLimit: 5
  minReadySeconds: 2
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0
  selector:
    matchLabels:
      app: {{.ProjectName}}
  template:
    metadata:
      labels:
        app: {{.ProjectName}}
    spec:
      containers:
        - name: {{.ProjectName}}
          image: <IMAGE>
          imagePullPolicy: IfNotPresent
          resources:
            limits:
              memory: 4096Mi
              cpu: 4000m
            requests:
              memory: 256Mi
              cpu: 250m
          env:
            - name: APP_NAME
              value: {{.ProjectName}}
            - name: DEPLOY_ENV
              value: kubernetes
            - name: RUN_ENV
              value: <RUN_ENV>
            - name: DISCOVERY_SERVER_GROUP
              value: <DISCOVERY_SERVER_GROUP>
            - name: DISCOVERY_SERVER_NAME
              value: <DISCOVERY_SERVER_NAME>
            - name: DISCOVERY_SERVER_PORT
              value: <DISCOVERY_SERVER_PORT>
            - name: DISCOVERY_SERVER_VERIFY_DATA
              value: <DISCOVERY_SERVER_VERIFY_DATA>
          livenessProbe:
            tcpSocket:
              port: 8000
            initialDelaySeconds: 2
            timeoutSeconds: 1
            periodSeconds: 1
            successThreshold: 1
            failureThreshold: 3
          readinessProbe:
            tcpSocket:
              port: 8000
            initialDelaySeconds: 2
            timeoutSeconds: 1
            periodSeconds: 1
            successThreshold: 1
            failureThreshold: 3
          ports:
            - name: web
              containerPort: 8000
              protocol: TCP
            - name: rpc
              containerPort: 9000
              protocol: TCP
      imagePullSecrets:
        - name: {{.NameSpace}}-secret
---
apiVersion: autoscaling/v2beta2
kind: HorizontalPodAutoscaler
metadata:
  name: {{.ProjectName}}-hpa
  namespace: {{.NameSpace}}
spec:
  scaleTargetRef:   
    apiVersion: apps/v1
    kind: Deployment  
    name: {{.ProjectName}}-deployment
  maxReplicas: 10
  minReplicas: 1
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 80{{ if .NeedService }}
---
apiVersion: v1
kind: Service
metadata:
  name: {{.ProjectName}}-service
  namespace: {{.NameSpace}}
  labels:
    app: {{.ProjectName}}
spec:
  type: ClusterIP
  ports:
  - name: web
    protocol: TCP
    port: 80
    targetPort: 8000
  - name: rpc
    protocol: TCP
    port: 90
    targetPort: 9000
  selector:
    app: {{.ProjectName}}{{ end }}{{ if .NeedIngress}}
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: {{.ProjectName}}-ingress
  namespace: {{.NameSpace}}
spec:
  rules: 
  - host: {{.HostName}}
    http:
      paths:
      - path: /
        backend:
          serviceName: {{.ProjectName}}-service
          serviceport: 80{{ end }}`

const path = "./"
const dockerfilename = "Dockerfile"
const deploymentname = "deployment.yaml"

var dockerfiletml *template.Template
var dockerfilefile *os.File

var deploymenttml *template.Template
var deploymentfile *os.File

func init() {
	var e error
	dockerfiletml, e = template.New("dockerfile").Parse(dockerfiletext)
	if e != nil {
		panic(fmt.Sprintf("create template for %s error:%s", path+dockerfilename, e))
	}

	deploymenttml, e = template.New("deployment").Parse(deploymenttext)
	if e != nil {
		panic(fmt.Sprintf("create template for %s error:%s", path+deploymentname, e))
	}
}
func CreatePathAndFile() {
	var e error
	if e = os.MkdirAll(path, 0755); e != nil {
		panic(fmt.Sprintf("make dir:%s error:%s", path, e))
	}
	dockerfilefile, e = os.OpenFile(path+dockerfilename, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic(fmt.Sprintf("make file:%s error:%s", path+dockerfilename, e))
	}

	deploymentfile, e = os.OpenFile(path+deploymentname, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic(fmt.Sprintf("make file:%s error:%s", path+deploymentname, e))
	}
}

type data struct {
	ProjectName string
	NameSpace   string
	NeedService bool
	NeedIngress bool
	HostName    string
}

func Execute(projectname string, namespace string, needservice bool, needingress bool, hostname string) {
	if e := dockerfiletml.Execute(dockerfilefile, projectname); e != nil {
		panic(fmt.Sprintf("write content into file:%s from template error:%s", path+dockerfilename, e))
	}

	if e := deploymenttml.Execute(deploymentfile, &data{ProjectName: projectname, NameSpace: namespace, NeedService: needservice, NeedIngress: needingress, HostName: hostname}); e != nil {
		panic(fmt.Sprintf("write content into file:%s from template error:%s", path+deploymentname, e))
	}
}
