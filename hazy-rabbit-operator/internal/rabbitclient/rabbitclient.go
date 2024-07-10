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

type SmoreQueueCountError struct {
	s string
}

func (e *SmoreQueueCountError) Error() string {
	return e.s
}

type HttpError struct {
	StatusCode int
}

func (e *HttpError) Error() string {
	return fmt.Sprintf("Invalid status %v.", e.StatusCode)
}

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
func (client *RabbitClient) deleteQueue(queue string, vhost string) (*http.Response, error) {
	url := fmt.Sprintf("http://%s/api/queues/%s/%s", client.Endpoint, vhost, queue)
	return client.doHttpRequest(url, http.MethodDelete, nil, true)
}

func (client *RabbitClient) createExchange(e *RabbitExchange, ch *amqp.Channel) error {
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

	if err != nil {
		client.Log.Error(err, "Could not declare the exchange", "exchange", e.ExchangeName)
		return err
	}

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
		if err != nil {
			client.Log.Error(err, "Could not declare the queue", "queue", q)
			return err
		}

		err = ch.QueueBind(
			q,              // queue name
			"",             // routing key
			e.ExchangeName, // exchange
			false,
			nil,
		)
		if err != nil {
			client.Log.Error(err, "Could not declare bind queue to exchange", "queue", q, "exchnage", e.ExchangeName)
			return err
		}

	}
	return nil
}

func (client *RabbitClient) createAuthz(e *RabbitExchange, user *RabbitUser) error {

	client.Log.Info("Entering RabbitClient.createAuthz")

	// create vhost
	client.Log.Info("Creating VHost", "vHost", e.VHost)

	vHostCreateUrl := fmt.Sprintf("http://%s/api/vhosts/%s", client.Endpoint, e.VHost)
	_, err := client.doHttpRequest(vHostCreateUrl, http.MethodPut, nil, true)

	if err != nil {
		client.Log.Error(err, "Could not create vhost", "vHost", e.VHost)
		return err
	}

	// create user
	client.Log.Info("Creating User", "user", user.Username)
	userCreateUrl := fmt.Sprintf("http://%s/api/users/%s", client.Endpoint, user.Username)
	userBody, err := json.Marshal(map[string]string{"password": user.Password, "tags": "management"})
	if err != nil {
		client.Log.Error(err, "Could not marshall User PUT body")
		return err
	}
	_, err = client.doHttpRequest(userCreateUrl, http.MethodPut, userBody, true)

	if err != nil {
		client.Log.Error(err, "Could not create user", "user", user.Username)
		return err

	}

	// attach read/write only permission on vhost
	client.Log.Info("Attaching Permissions to User/VHost.", "user", user.Username, "vHost", e.VHost)
	permissionAttachUrl := fmt.Sprintf("http://%s/api/permissions/%s/%s", client.Endpoint, e.VHost, user.Username)
	userPermissions, err := json.Marshal(map[string]string{"configure": "", "write": ".*", "read": ".*"})
	if err != nil {

		client.Log.Error(err, "Could not marshall user permission attach PUT body")
		return err
	}

	_, err = client.doHttpRequest(permissionAttachUrl, http.MethodPut, userPermissions, true)

	if err != nil {
		client.Log.Error(err, "Could not attach permissions user in vhost ", "user", user.Username, "vHost", e.VHost)
		return err
	}

	return nil

}

func (client *RabbitClient) deleteAuthz(e *RabbitExchange, user *RabbitUser) error {

	client.Log.Info("Entering RabbitClient.deleteAuthz func.")

	// delete vhost.
	deleteVHostUrl := fmt.Sprintf("http://%s/api/vhosts/%s", client.Endpoint, e.VHost)

	res, err := client.doHttpRequest(deleteVHostUrl, http.MethodDelete, nil, true)
	if err != nil {
		if res.StatusCode == http.StatusNotFound {
			client.Log.Info("VHost does not exist", "vHost", e.VHost)
		} else {
			client.Log.Error(err, "Could not delete vhost", "vHost", e.VHost)
			return err

		}
	}

	// delete user.
	deleteUserUrl := fmt.Sprintf("http://%s/api/users/%s", client.Endpoint, user.Username)
	client.doHttpRequest(deleteUserUrl, http.MethodDelete, nil, true)
	if err != nil {
		if res.StatusCode == http.StatusNotFound {
			client.Log.Info("User does not exist", "user", user.Username)
		} else {
			client.Log.Error(err, "Could not delete user", "user", user.Username)
			return err
		}
	}
	return nil

}

func (client *RabbitClient) listVHosts() ([]RabbitVHost, error) {

	listVHostsUrl := fmt.Sprintf("http://%s/api/vhosts", client.Endpoint)
	res, err := client.doHttpRequest(listVHostsUrl, http.MethodGet, nil, true)

	if err != nil {
		client.Log.Error(err, "Could not list vHosts")
		return make([]RabbitVHost, 0), err
	}

	var vhosts []RabbitVHost

	body, err := ioutil.ReadAll(res.Body)

	if err != nil {
		client.Log.Error(err, "Failed to read response for list vhosts.")
		return make([]RabbitVHost, 0), err
	}

	err = json.Unmarshal(body, &vhosts)

	if err != nil {
		client.Log.Error(err, "Failed to unmarshall response response for list vhosts.")
		return make([]RabbitVHost, 0), err
	}

	return vhosts, nil

}

func (client *RabbitClient) listQueues(vHost string) ([]RabbitQueue, error) {
	listQueuesUrl := fmt.Sprintf("http://%s/api/queues/%s", client.Endpoint, vHost)
	res, err := client.doHttpRequest(listQueuesUrl, http.MethodGet, nil, true)

	if err != nil {
		client.Log.Error(err, "Could not list queues for vhost", "vHost", vHost)
		return make([]RabbitQueue, 0), err
	}

	var queues []RabbitQueue

	body, err := ioutil.ReadAll(res.Body)

	if err != nil {
		client.Log.Error(err, "Failed to read response for list queues for vHost.", "vHost", vHost)
		return make([]RabbitQueue, 0), err
	}

	err = json.Unmarshal(body, &queues)

	if err != nil {
		client.Log.Error(err, "Failed to unmarshall response for list queues for vHost.", "vHost", vHost)
		return make([]RabbitQueue, 0), err
	}

	return queues, nil

}

func (client *RabbitClient) SetupRabbit(re *RabbitExchange) (RabbitUser, error) {
	client.Log.Info("Setting up Hazy Zone")

	// build client from RabbitExchange.
	clientuser := buildClientUser(re)

	// checking if VHost already exists.
	vhosts, err := client.listVHosts()

	if err != nil {
		client.Log.Error(err, "Failed to list vhosts.")
		return RabbitUser{}, err
	}

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
		client.Log.Info("VHost ", re.VHost, " does not exists, creating vhost/user")

		err = client.createAuthz(re, &clientuser)
		if err != nil {
			client.Log.Error(err, "Failed to create user/vhosts.")
			return RabbitUser{}, err
		}
	} else {
		client.Log.Info("VHost ", re.VHost, " already exists, no need to create vhost/user")
	}

	curr_queues, err := client.listQueues(re.VHost)

	if err != nil {
		client.Log.Error(err, "Failed to list queues.")
		return RabbitUser{}, err
	}

	curr_queues_map := make(map[string]string)
	for _, curr_queue := range curr_queues {
		curr_queues_map[curr_queue.Name] = curr_queue.Name
	}
	wanted_queues_map := make(map[string]string)
	for _, wanted_queue := range re.Queues {
		wanted_queues_map[wanted_queue] = wanted_queue
	}

	queuesToDelete := make([]string, 0)

	for k, _ := range curr_queues_map {
		_, ok := wanted_queues_map[k]
		if !ok {
			queuesToDelete = append(queuesToDelete, k)
		}
	}

	conn, err := amqp.Dial(client.buildAMQPUrl(re.VHost))
	defer conn.Close()

	if err != nil {
		client.Log.Error(err, "Failed to connect to RabbitMQ")
		return RabbitUser{}, err
	}

	ch, err := conn.Channel()
	defer ch.Close()
	if err != nil {
		client.Log.Error(err, "Failed to open a Channel  RabbitMQ")
		return RabbitUser{}, err
	}

	// exchange, queuesToAdd
	client.Log.Info("Upserting Exchange for vHost", "vHost", re.VHost)
	err = client.createExchange(re, ch)
	if err != nil {
		client.Log.Error(err, "Failed to create exchange")
		return RabbitUser{}, err
	}

	if len(queuesToDelete) > 0 {
		for _, queue := range queuesToDelete {
			res, err := client.deleteQueue(queue, re.VHost)

			if err != nil {
				if res.StatusCode == http.StatusNotFound {
					client.Log.Info("Queue does not exist", "queue", queue)
				} else {
					client.Log.Error(err, "Failed to delete queue", "queue", queue)
					return RabbitUser{}, err
				}

			}

		}
	}

	res_queues, err := client.listQueues(re.VHost)
	if err != nil {
		client.Log.Error(err, "Failed to list queues for validation")
		return RabbitUser{}, err
	}

	if len(res_queues) != len(wanted_queues_map) {
		if err != nil {
			client.Log.Error(err, "Failed to list queues for validation")
			return RabbitUser{}, &SmoreQueueCountError{fmt.Sprintf("Not all queues were created, Expected: %v, Actual: %v", wanted_queues_map, res_queues)}
		}
	}

	client.Log.Info("Set up of Zone complete")
	return clientuser, nil

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

func (client *RabbitClient) TearDownRabbit(re *RabbitExchange) error {
	// tears down vhost, username, exchange and queues.
	clientuser := buildClientUser(re)

	// vhost, user
	err := client.deleteAuthz(re, &clientuser)

	if err != nil {
		client.Log.Error(err, "Failed to delete the vhost/user", "vHost", re.VHost)
		return err
	}
	return nil
}

func (client *RabbitClient) buildAMQPUrl(vHost string) string {
	return fmt.Sprintf("amqp://%s:%s@%s/%s", client.AdminUser.Username, client.AdminUser.Password, buildRabbitEndpoint(), vHost)
}

func (client *RabbitClient) doHttpRequest(url string, method string, body []byte, shouldFail bool) (*http.Response, error) {
	httpclient := &http.Client{}

	var reader io.Reader

	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url, reader)

	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(client.AdminUser.Username, client.AdminUser.Password)

	res, err := httpclient.Do(req)

	if err != nil {
		return nil, err
	}

	if !(res.StatusCode >= 200 && res.StatusCode < 300) {
		return res, &HttpError{res.StatusCode}
	}

	return res, nil
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
	_, err := client.doHttpRequest(overiewUrl, http.MethodGet, nil, false)

	if err != nil {
		return nil
	}

	return &client
}
