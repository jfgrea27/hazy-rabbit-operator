package main

import (
	"fmt"
	"log"
	"os"

	amqp "github.com/rabbitmq/amqp091-go"
)

func failOnError(err error, msg string) {
	if err != nil {
		log.Panicf("%s: %s", msg, err)
	}
}

func main() {
	conn, err := amqp.Dial(
		fmt.Sprintf(
			"amqp://%v:%v@%v:%v/%v",
			os.Getenv("RABBIT_USER"),
			os.Getenv("RABBIT_PASSWORD"),
			os.Getenv("SAMPLE_RABBIT_HOST"),
			os.Getenv("SAMPLE_RABBIT_PORT"),
			os.Getenv("SAMPLE_EXCHANGE"),
		),
	)
	failOnError(err, "Failed to connect to RabbitMQ")
	defer conn.Close()

	ch, err := conn.Channel()
	failOnError(err, "Failed to open a channel")
	defer ch.Close()

	msgs, err := ch.Consume(
		os.Getenv("SAMPLE_QUEUE"), // queue
		"",                        // consumer
		true,                      // auto-ack
		false,                     // exclusive
		false,                     // no-local
		false,                     // no-wait
		nil,                       // args
	)
	failOnError(err, "Failed to register a consumer")

	var forever chan struct{}

	go func() {
		for d := range msgs {
			log.Printf("Received a message: %s", d.Body)
		}
	}()

	log.Printf(" [*] Waiting for messages. To exit press CTRL+C")
	<-forever

}
