package messaging

import (
	"fmt"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

type RabbitMQ struct {
	conn *amqp.Connection
}

type Consumer struct {
	channel    *amqp.Channel
	queue      amqp.Queue
	deliveries <-chan amqp.Delivery
	done       chan bool
	tag        string
}

func NewRabbitMQ(url string) (*RabbitMQ, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	return &RabbitMQ{conn: conn}, nil
}

func (r *RabbitMQ) Close() error {
	return r.conn.Close()
}

func (r *RabbitMQ) CreateTenantQueue(tenantID string) (*Consumer, error) {
	ch, err := r.conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	queueName := fmt.Sprintf("tenant_%s_queue", tenantID)
	
	queue, err := ch.QueueDeclare(
		queueName, // name
		true,      // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	if err != nil {
		ch.Close()
		return nil, fmt.Errorf("failed to declare queue: %w", err)
	}

	// Create dead letter queue for failed messages
	dlqName := fmt.Sprintf("tenant_%s_dlq", tenantID)
	_, err = ch.QueueDeclare(
		dlqName,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		ch.Close()
		return nil, fmt.Errorf("failed to declare dead letter queue: %w", err)
	}

	consumerTag := fmt.Sprintf("consumer_%s", tenantID)
	deliveries, err := ch.Consume(
		queue.Name,  // queue
		consumerTag, // consumer
		false,       // auto-ack
		false,       // exclusive
		false,       // no-local
		false,       // no-wait
		nil,         // args
	)
	if err != nil {
		ch.Close()
		return nil, fmt.Errorf("failed to register consumer: %w", err)
	}

	return &Consumer{
		channel:    ch,
		queue:      queue,
		deliveries: deliveries,
		done:       make(chan bool),
		tag:        consumerTag,
	}, nil
}

func (r *RabbitMQ) DeleteTenantQueue(tenantID string) error {
	ch, err := r.conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open channel: %w", err)
	}
	defer ch.Close()

	queueName := fmt.Sprintf("tenant_%s_queue", tenantID)
	dlqName := fmt.Sprintf("tenant_%s_dlq", tenantID)

	// Delete main queue
	_, err = ch.QueueDelete(queueName, false, false, false)
	if err != nil {
		log.Printf("Warning: failed to delete queue %s: %v", queueName, err)
	}

	// Delete dead letter queue
	_, err = ch.QueueDelete(dlqName, false, false, false)
	if err != nil {
		log.Printf("Warning: failed to delete DLQ %s: %v", dlqName, err)
	}

	return nil
}

func (r *RabbitMQ) PublishMessage(tenantID string, payload []byte) error {
	ch, err := r.conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open channel: %w", err)
	}
	defer ch.Close()

	queueName := fmt.Sprintf("tenant_%s_queue", tenantID)

	err = ch.Publish(
		"",        // exchange
		queueName, // routing key
		false,     // mandatory
		false,     // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        payload,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	return nil
}

func (c *Consumer) Start(handler func([]byte) error) {
	go func() {
		for {
			select {
			case delivery := <-c.deliveries:
				if err := handler(delivery.Body); err != nil {
					log.Printf("Failed to process message: %v", err)
					delivery.Nack(false, false) // Send to DLQ
				} else {
					delivery.Ack(false)
				}
			case <-c.done:
				return
			}
		}
	}()
}

func (c *Consumer) Stop() error {
	close(c.done)
	
	// Cancel consumer
	if err := c.channel.Cancel(c.tag, false); err != nil {
		log.Printf("Warning: failed to cancel consumer: %v", err)
	}

	return c.channel.Close()
}