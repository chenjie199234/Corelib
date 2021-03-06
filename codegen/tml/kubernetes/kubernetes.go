package kubernetes

import (
	"fmt"
	"os"
	"text/template"
)

const dockerfiletext = `FROM debian:stable-slim
RUN apt-get update && apt-get install -y ca-certificates && mkdir /root/app && mkdir /root/app/k8sconfig && mkdir /root/app/remoteconfig
WORKDIR /root/app
COPY main probe.sh AppConfig.json SourceConfig.json ./
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
            - name: DEPLOY_ENV
              value: kubernetes
            - name: RUN_ENV
              value: <RUN_ENV>
            - name: SERVER_VERIFY_DATA
              value: <SERVER_VERIFY_DATA>
            - name: CONFIG_TYPE
              value: <CONFIG_TYPE>
          livenessProbe:
            exec:
              command:
                - ./probe.sh
            initialDelaySeconds: 2
            timeoutSeconds: 1
            periodSeconds: 1
            successThreshold: 1
            failureThreshold: 3
          readinessProbe:
            exec:
              command:
                - ./probe.sh
            initialDelaySeconds: 2
            timeoutSeconds: 1
            periodSeconds: 1
            successThreshold: 1
            failureThreshold: 3
          volumeMounts:
            - name: {{.ProjectName}}-config-volume
              mountPath: /root/app/k8sconfig
      volumes:
        - name: {{.ProjectName}}-config-volume
          configMap:
            name: {{.ProjectName}}-config
            items:
              - key: app-config
                path: AppConfig.json
              - key: source-config
                path: SourceConfig.json
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
  name: {{.ProjectName}}-service{{ if .HeadlessService }}-headless{{ end }}
  namespace: {{.NameSpace}}
  labels:
    app: {{.ProjectName}}
spec:
  type: ClusterIP{{ if .HeadlessService }}
  clusterIP: None{{ end }}
  ports:
  - name: web
    protocol: TCP
    port: 80
    targetPort: 8000
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
      - path: /{{.ProjectName}}
        backend:
          serviceName: {{.ProjectName}}-service
          servicePort: 80{{ end }}`

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
		panic(fmt.Sprintf("create template error:%s", e))
	}

	deploymenttml, e = template.New("deployment").Parse(deploymenttext)
	if e != nil {
		panic(fmt.Sprintf("create template error:%s", e))
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
	ProjectName     string
	NameSpace       string
	NeedService     bool
	HeadlessService bool
	NeedIngress     bool
	HostName        string
}

func Execute(projectname string, namespace string, needservice bool, needheadlessservice bool, needingress bool, hostname string) {
	if e := dockerfiletml.Execute(dockerfilefile, projectname); e != nil {
		panic(fmt.Sprintf("write content into file:%s error:%s", path+dockerfilename, e))
	}
	tempdata := &data{
		ProjectName:     projectname,
		NameSpace:       namespace,
		NeedService:     needservice,
		HeadlessService: needheadlessservice,
		NeedIngress:     needingress,
		HostName:        hostname,
	}
	if e := deploymenttml.Execute(deploymentfile, tempdata); e != nil {
		panic(fmt.Sprintf("write content into file:%s error:%s", path+deploymentname, e))
	}
}
