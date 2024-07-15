

docker_sample_build:
	go build -o bin/rabbit-sample-consumer dev/consumer/consumer.go
	docker build -t rabbit-sample-consumer -f dev/consumer/Dockerfile .
