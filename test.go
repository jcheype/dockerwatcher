package main

import (
	"github.com/samalba/dockerclient"
	"log"
	"os"
	"text/template"
	"strings"
	"io/ioutil"
	"os/signal"
	"syscall"
	"strconv"
	"bufio"
)

func eventCallback(event *dockerclient.Event, args ...interface{}) {
	log.Printf("Received event: %#v\n", *event)
	if docker, ok := args[0].(*dockerclient.DockerClient); ok {
		generateAll(docker)
	}
}

type Vhost struct {
	Name  string
	Names string
	Host  string
	Port  int
}

func cleanAllAuto() {
	files, _ := ioutil.ReadDir(os.Getenv("SITE"))
	for _, f := range files {
		if (strings.HasPrefix(f.Name(), "docker_auto_")) {
			os.Remove(os.Getenv("SITE") + "/" + f.Name())
		}
	}
}

func generateVHost(container dockerclient.Container, port dockerclient.Port) {
	tmpl, _ := template.New("vhost").Parse(templateRaw)
	names := strings.Replace(strings.Join(container.Names, ","), "/", "", -1)
	name := strings.Replace(container.Names[len(container.Names)-1], "/", "", -1)
	vhost := Vhost{name, names, os.Getenv("HOST"), port.PublicPort}

	fo, _ := os.Create(os.Getenv("SITE") + "/docker_auto_" + name)

	tmpl.Execute(fo, vhost)

	fo.Close()
	log.Printf("docker_auto_" + name)
}

func generateAll(docker *dockerclient.DockerClient){
	// Get only running containers
	containers, err := docker.ListContainers(false)
	if err != nil {
		log.Fatal(err)
	}

	cleanAllAuto()
	for _, c := range containers {
		log.Println(c.Id, c.Names)
		for _, port := range c.Ports {
			if (port.PrivatePort == 80 || port.PrivatePort == 8080) {
				generateVHost(c, port)
				break
			}
		}
	}

	if file, err := os.Open(PID_FILE); err == nil {
		scanner := bufio.NewScanner(file)
		scanner.Scan()
		value := string(scanner.Text())
		pid, _ := strconv.Atoi(value)
		log.Printf("SIGHUP: %d", pid)
		syscall.Kill(pid, syscall.SIGHUP)
	}

}

func main() {
	if (os.Getenv("SITE") == "") {
		os.Setenv("SITE", ".")
	}
	if (os.Getenv("HOST") == "") {
		os.Setenv("HOST", "docker")
	}
	if (os.Getenv("DOCKER_HOST") == "") {
		os.Setenv("DOCKER_HOST", "unix:///var/run/docker.sock")
	}

	host := strings.Replace(os.Getenv("DOCKER_HOST"), "tcp://", "http://", 1)
	docker, _ := dockerclient.NewDockerClient(host)

	generateAll(docker)

	docker.StartMonitorEvents(eventCallback, docker)

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP)
	go func() {
		for sig := range c {
			log.Printf("captured %v, reload", sig)
			generateAll(docker)
		}
	}()

	for { select {} }

}

const (
	templateRaw = `server {
		listen 80;
		server_name {{.Names}}.{{.Host}};

		#proxy_pass_header Server;
		#server_tokens off;

		access_log /var/log/nginx/{{.Name}}.access.log;
		location / {
			proxy_pass http://{{.Host}}:{{.Port}}/;
			proxy_next_upstream error timeout invalid_header http_500 http_502 http_503 http_504;
			proxy_redirect off;
			proxy_buffering off;

			client_max_body_size 0; # disable any limits to avoid HTTP 413 for large image uploads

			proxy_set_header Host $host;
			proxy_set_header X-Real-IP $remote_addr;
			proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
			index  index.html index.htm;
		}
	}`
	PID_FILE    = "/var/run/nginx.pid"
)
