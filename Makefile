

docker_sample_build:
	GOOS=linux GOARCH=amd64 go build -o bin/rabbit-sample-consumer dev/consumer/consumer.go
	docker build -t rabbit-sample-consumer -f dev/consumer/Dockerfile .
