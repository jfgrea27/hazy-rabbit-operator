package rabbitclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/go-logr/logr"
	amqp "github.com/rabbitmq/amqp091-go"
)

type RabbitExchange struct {
	ExchangeName string   `json:"exchangeName"`
	Queues       []string `json:"queues"`
	VHost        string   `json:"vhost"`
	Password     string   `json:"password"`
}

type RabbitUser struct {
	Username string
	Password string
}

type RabbitClient struct {
	Log       logr.Logger
	Endpoint  string
	AdminUser RabbitUser
}

type RabbitVHost struct {
	Name string `json:"name"`
}

type RabbitQueue struct {
	Name  string `json:"name"`
	VHost string `json:"vhost"`
}

func failOnError(err error, msg string, log logr.Logger) {
	if err != nil {
		log.Error(err, msg)
	}
}

func buildConsoleEndpoint() string {
	host := os.Getenv("RABBIT_HOST")
	port := os.Getenv("RABBIT_CONSOLE_PORT")

	return fmt.Sprintf("%s:%s", host, port)
}

func buildRabbitEndpoint() string {
	host := os.Getenv("RABBIT_HOST")
	port := os.Getenv("RABBIT_PORT")

	return fmt.Sprintf("%s:%s", host, port)
}

func buildAdminUser() RabbitUser {
	admin_username := os.Getenv("RABBIT_ADMIN_USERNAME")
	admin_password := os.Getenv("RABBIT_ADMIN_PASSWORD")

	return RabbitUser{
		admin_username,
		admin_password,
	}
}

func buildClientUser(re *RabbitExchange) RabbitUser {
	return RabbitUser{
		Username: re.VHost,
		Password: re.Password,
	}
}
func (client *RabbitClient) deleteQueue(queue string, vhost string) {
	url := fmt.Sprintf("http://%s/api/queues/%s/%s", client.Endpoint, vhost, queue)
	client.doHttpRequest(url, http.MethodDelete, nil, true)
}

func (client *RabbitClient) createExchange(e *RabbitExchange, ch *amqp.Channel) {
	// declare exchange
	err := ch.ExchangeDeclare(
		e.ExchangeName, // name
		"direct",       // type
		true,           // durable
		false,          // auto-deleted
		false,          // internal
		false,          // no-wait
		nil,            // arguments
	)

	failOnError(err, fmt.Sprintf("Failed to declare exchange %s", e.ExchangeName), client.Log)

	// create queues and bind queues
	for _, q := range e.Queues {
		_, err := ch.QueueDeclare(
			q,     // name
			false, // durable
			false, // delete when unused
			false, // exclusive
			false, // no-wait
			nil,   // arguments
		)
		failOnError(err, fmt.Sprintf("Failed to declare queue %s", q), client.Log)

		err = ch.QueueBind(
			q,              // queue name
			"",             // routing key
			e.ExchangeName, // exchange
			false,
			nil,
		)
		failOnError(err, fmt.Sprintf("Failed to bind queue %s to exchange %s", q, e.ExchangeName), client.Log)
	}
}

func (client *RabbitClient) createAuthz(e *RabbitExchange, user *RabbitUser) {

	client.Log.Info("Entering RabbitClient.createAuthz")

	// create vhost
	client.Log.Info(fmt.Sprintf("Creating VHost %v", e.VHost))
	vHostCreateUrl := fmt.Sprintf("http://%s/api/vhosts/%s", client.Endpoint, e.VHost)
	client.doHttpRequest(vHostCreateUrl, http.MethodPut, nil, true)

	// create user
	client.Log.Info(fmt.Sprintf("Creating User %v", user.Username))
	userCreateUrl := fmt.Sprintf("http://%s/api/users/%s", client.Endpoint, user.Username)
	fmt.Print(userCreateUrl)
	userBody, err := json.Marshal(map[string]string{"password": user.Password, "tags": "management"})

	if err != nil {
		failOnError(err, "Could not marshall user PUT body", client.Log)
	}

	client.doHttpRequest(userCreateUrl, http.MethodPut, userBody, true)

	// attach read/write only permission on vhost
	client.Log.Info(fmt.Sprintf("Attaching Permissions to User %v in VHost %v.", user.Username, e.VHost))
	permissionAttachUrl := fmt.Sprintf("http://%s/api/permissions/%s/%s", client.Endpoint, e.VHost, user.Username)
	userPermissions, err := json.Marshal(map[string]string{"configure": "", "write": ".*", "read": ".*"})

	if err != nil {
		failOnError(err, "Could not marshall user permission attach PUT body", client.Log)
	}

	client.doHttpRequest(permissionAttachUrl, http.MethodPut, userPermissions, true)

}

func (client *RabbitClient) deleteAuthz(e *RabbitExchange, user *RabbitUser) {

	client.Log.Info("Entering RabbitClient.deleteAuthz func.")

	// delete vhost.
	deleteVHostUrl := fmt.Sprintf("http://%s/api/vhosts/%s", client.Endpoint, e.VHost)
	client.doHttpRequest(deleteVHostUrl, http.MethodDelete, nil, true)

	// delete user.
	deleteUserUrl := fmt.Sprintf("http://%s/api/users/%s", client.Endpoint, user.Username)
	client.doHttpRequest(deleteUserUrl, http.MethodDelete, nil, true)
}

func (client *RabbitClient) listVHosts() []RabbitVHost {

	listVHostsUrl := fmt.Sprintf("http://%s/api/vhosts", client.Endpoint)
	res, _ := client.doHttpRequest(listVHostsUrl, http.MethodGet, nil, true)

	var vhosts []RabbitVHost

	body, err := ioutil.ReadAll(res.Body)

	if err != nil {
		failOnError(err, "Failed to read response for list vhosts.", client.Log)
	}

	err = json.Unmarshal(body, &vhosts)

	if err != nil {
		failOnError(err, "Failed to unmarshall response for list vhosts.", client.Log)
	}

	return vhosts

}

func (client *RabbitClient) listQueues(vHost string) []RabbitQueue {
	listQueuesUrl := fmt.Sprintf("http://%s/api/queues/%s", client.Endpoint, vHost)
	res, _ := client.doHttpRequest(listQueuesUrl, http.MethodGet, nil, true)

	var queues []RabbitQueue

	body, err := ioutil.ReadAll(res.Body)

	if err != nil {
		failOnError(err, "Failed to read response for list queues.", client.Log)
	}

	err = json.Unmarshal(body, &queues)

	if err != nil {
		failOnError(err, "Failed to unmarshall response for list queues.", client.Log)
	}

	return queues

}

func (client *RabbitClient) SetupRabbit(re *RabbitExchange) RabbitUser {
	client.Log.Info("Setting up Hazy Zone")

	// build client from RabbitExchange.
	clientuser := buildClientUser(re)

	// checking if VHost already exists.
	vhosts := client.listVHosts()
	vHostAlreadyExists := func() bool {
		for _, vhost := range vhosts {
			if vhost.Name == re.VHost {
				return true
			}
		}
		return false

	}()

	if !vHostAlreadyExists {
		// create vhost and user as not exists
		client.Log.Info(fmt.Sprintf("VHost %v does not exists, creating vhost/user", re.VHost))

		client.createAuthz(re, &clientuser)
	} else {
		client.Log.Info(fmt.Sprintf("VHost %v already exists, no need to create vhost/user", re.VHost))
	}

	curr_queues_map := make(map[string]string)
	for _, curr_queue := range client.listQueues(re.VHost) {
		curr_queues_map[curr_queue.Name] = curr_queue.Name
	}
	wanted_queues_map := make(map[string]string)
	for _, wanted_queue := range re.Queues {
		wanted_queues_map[wanted_queue] = wanted_queue
	}

	queuesToDelete := make([]string, len(curr_queues_map))

	for k, _ := range curr_queues_map {
		_, ok := wanted_queues_map[k]
		if !ok {
			queuesToDelete = append(queuesToDelete, k)
		}
	}

	conn, err := amqp.Dial(client.buildAMQPUrl(re.VHost))
	failOnError(err, "Failed to connect to RabbitMQ", client.Log)

	ch, err := conn.Channel()

	// exchange, queuesToAdd
	client.createExchange(re, ch)

	if len(queuesToDelete) > 0 {
		client.Log.Info(fmt.Sprintf("Deleting %v queues", len(queuesToDelete)))
		for _, queue := range queuesToDelete {
			client.deleteQueue(queue, re.VHost)
		}
	}

	res_queues := client.listQueues(re.VHost)

	if len(res_queues) != len(wanted_queues_map) {
		failOnError(err, fmt.Sprintf("Not all queues were created, Expected: %v, Actual: %v", wanted_queues_map, res_queues), client.Log)
	}

	failOnError(err, "Failed to open a channel", client.Log)

	defer ch.Close()
	defer conn.Close()

	return clientuser

}

func getListQueuesToDelete(curr map[string]string, wanted map[string]string) []string {
	res := make([]string, 0)

	for k, _ := range curr {
		_, ok := wanted[k]
		if !ok {
			res = append(res, k)
		}
	}

	return res
}

func buildStringsMap(s []string) map[string]string {
	res := make(map[string]string)
	for _, v := range s {
		res[v] = v
	}
	return res
}

func (client *RabbitClient) TearDownRabbit(re *RabbitExchange) {
	// tears down vhost, username, exchange and queues.
	clientuser := buildClientUser(re)

	// vhost, user
	client.deleteAuthz(re, &clientuser)
}

func (client *RabbitClient) buildAMQPUrl(vHost string) string {
	return fmt.Sprintf("amqp://%s:%s@%s/%s", client.AdminUser.Username, client.AdminUser.Password, buildRabbitEndpoint(), vHost)
}

func (client *RabbitClient) doHttpRequest(url string, method string, body []byte, shouldFail bool) (*http.Response, bool) {
	httpclient := &http.Client{}

	var reader io.Reader

	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url, reader)

	if err != nil {
		if shouldFail {
			failOnError(err, fmt.Sprintf("Failed to %v request for %v.", method, url), client.Log)
		} else {
			return nil, false
		}
	}

	req.SetBasicAuth(client.AdminUser.Username, client.AdminUser.Password)

	res, err := httpclient.Do(req)

	if err != nil {
		if shouldFail {

			failOnError(err, fmt.Sprintf("Failed to run request for %v.", url), client.Log)
		} else {
			return nil, false
		}
	}

	if res.StatusCode != 200 {
		msg := fmt.Sprintf("Invalid status %v.", res.StatusCode)
		if shouldFail {
			failOnError(err, msg, client.Log)
		} else {
			client.Log.Info(msg)
			return nil, false
		}

	}
	return res, true
}

func BuildRabbitClient(log logr.Logger) *RabbitClient {

	ep := buildConsoleEndpoint()

	admin := buildAdminUser()

	client := RabbitClient{
		Log:       log,
		Endpoint:  ep,
		AdminUser: admin,
	}

	// check rabbit can be pinged
	overiewUrl := fmt.Sprintf("http://%s/api/overview", client.Endpoint)
	_, ok := client.doHttpRequest(overiewUrl, http.MethodGet, nil, false)

	if !ok {
		return nil
	}

	return &client
}
