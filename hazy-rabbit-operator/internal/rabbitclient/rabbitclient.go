package rabbitclient

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/go-logr/logr"
	hazyv1alpha1 "github.com/jfgrea27/hazy-rabbit-operator/api/v1alpha1"
	hhttp "github.com/jfgrea27/hazy-rabbit-operator/internal/hhttp"
	amqp "github.com/rabbitmq/amqp091-go"
)

// Error type, created when invalid queues in RabbitMQ broker.
type SmokeQueueCountError struct {
	ExpectedQueues int
	ActualQueues   int
}

func (e *SmokeQueueCountError) Error() string {
	return fmt.Sprintf("Not all queues were created, Expected: %v, Actual: %v", e.ExpectedQueues, e.ActualQueues)

}

// Wrapper around any HTTP Error
type HttpError struct {
	StatusCode int
}

func (e *HttpError) Error() string {
	return fmt.Sprintf("Invalid status %v.", e.StatusCode)
}

type RabbitClient struct {
	Log  logr.Logger
	Http *hhttp.HttpClient
}

type RabbitVHost struct {
	Name string `json:"name"`
}

type RabbitQueue struct {
	Name  string `json:"name"`
	VHost string `json:"vhost"`
}

func (c *RabbitClient) deleteQueue(queue string, vhost string) (*http.Response, error) {
	url := fmt.Sprintf("api/queues/%s/%s", vhost, queue)
	return c.Http.DoHttpRequest(
		url,
		http.MethodDelete,
		nil,
	)
}

func (c *RabbitClient) createExchange(zs *hazyv1alpha1.HazyZoneSpec, ch *amqp.Channel) error {
	// declare exchange
	err := ch.ExchangeDeclare(
		zs.Exchange, // name
		"direct",    // type
		true,        // durable
		false,       // auto-deleted
		false,       // internal
		false,       // no-wait
		nil,         // arguments
	)

	if err != nil {
		c.Log.Error(err, "Could not declare the exchange", "exchange", zs.Exchange)
		return err
	}

	// create queues and bind queues
	for _, q := range zs.Queues {
		_, err := ch.QueueDeclare(
			q,     // name
			false, // durable
			false, // delete when unused
			false, // exclusive
			false, // no-wait
			nil,   // arguments
		)
		if err != nil {
			c.Log.Error(err, "Could not declare the queue", "queue", q)
			return err
		}

		err = ch.QueueBind(
			q,           // queue name
			"",          // routing key
			zs.Exchange, // exchange
			false,
			nil,
		)
		if err != nil {
			c.Log.Error(err, "Could not declare bind queue to exchange", "queue", q, "exchnage", zs.Exchange)
			return err
		}

	}
	return nil
}

func (c *RabbitClient) createAuthz(spec *hazyv1alpha1.HazyZoneSpec) error {

	c.Log.Info("Entering RabbitClient.createAuthz")

	// create vhost
	c.Log.Info("Creating VHost", "vHost", spec.VHost)

	vHostCreateUrl := fmt.Sprintf("api/vhosts/%s", spec.VHost)
	_, err := c.Http.DoHttpRequest(
		vHostCreateUrl,
		http.MethodPut,
		nil,
	)

	if err != nil {
		c.Log.Error(err, "Could not create vhost", "vHost", spec.VHost)
		return err
	}

	// create user
	c.Log.Info("Creating User", "user", spec.Username)
	userCreateUrl := fmt.Sprintf("api/users/%s", spec.Username)
	userBody, err := json.Marshal(map[string]string{"password": spec.Password, "tags": "management"})
	if err != nil {
		c.Log.Error(err, "Could not marshall User PUT body")
		return err
	}
	_, err = c.Http.DoHttpRequest(userCreateUrl, http.MethodPut, userBody)

	if err != nil {
		c.Log.Error(err, "Could not create user", "user", spec.Username)
		return err

	}

	// attach read/write only permission on vhost
	c.Log.Info("Attaching Permissions to User/VHost.", "user", spec.Username, "vHost", spec.VHost)
	permissionAttachUrl := fmt.Sprintf("api/permissions/%s/%s", spec.VHost, spec.Username)
	userPermissions, err := json.Marshal(map[string]string{"configure": "", "write": ".*", "read": ".*"})
	if err != nil {

		c.Log.Error(err, "Could not marshall user permission attach PUT body")
		return err
	}

	_, err = c.Http.DoHttpRequest(permissionAttachUrl, http.MethodPut, userPermissions)

	if err != nil {
		c.Log.Error(err, "Could not attach permissions user in vhost ", "user", spec.Username, "vHost", spec.VHost)
		return err
	}

	return nil

}

func (c *RabbitClient) deleteAuthz(spec *hazyv1alpha1.HazyZoneSpec) error {

	c.Log.Info("Entering RabbitClient.deleteAuthz func.")

	// delete vhost.
	deleteVHostUrl := fmt.Sprintf("api/vhosts/%s", spec.VHost)

	res, err := c.Http.DoHttpRequest(deleteVHostUrl, http.MethodDelete, nil)
	if err != nil {
		if res.StatusCode == http.StatusNotFound {
			c.Log.Info("VHost does not exist", "vHost", spec.VHost)
		} else {
			c.Log.Error(err, "Could not delete vhost", "vHost", spec.VHost)
			return err

		}
	}

	// delete user.
	deleteUserUrl := fmt.Sprintf("api/users/%s", spec.Username)
	c.Http.DoHttpRequest(deleteUserUrl, http.MethodDelete, nil)
	if err != nil {
		if res.StatusCode == http.StatusNotFound {
			c.Log.Info("User does not exist", "user", spec.Username)
		} else {
			c.Log.Error(err, "Could not delete user", "user", spec.Username)
			return err
		}
	}
	return nil

}

func (c *RabbitClient) listVHosts() ([]RabbitVHost, error) {

	res, err := c.Http.DoHttpRequest("api/vhosts", http.MethodGet, nil)

	if err != nil {
		c.Log.Error(err, "Could not list vHosts")
		return make([]RabbitVHost, 0), err
	}

	var vhosts []RabbitVHost

	body, err := io.ReadAll(res.Body)

	if err != nil {
		c.Log.Error(err, "Failed to read response for list vhosts.")
		return make([]RabbitVHost, 0), err
	}

	err = json.Unmarshal(body, &vhosts)

	if err != nil {
		c.Log.Error(err, "Failed to unmarshall response response for list vhosts.")
		return make([]RabbitVHost, 0), err
	}

	return vhosts, nil

}

func (c *RabbitClient) listQueues(vHost string) ([]RabbitQueue, error) {
	listQueuesUrl := fmt.Sprintf("api/queues/%s", vHost)
	res, err := c.Http.DoHttpRequest(listQueuesUrl, http.MethodGet, nil)

	if err != nil {
		c.Log.Error(err, "Could not list queues for vhost", "vHost", vHost)
		return make([]RabbitQueue, 0), err
	}

	var queues []RabbitQueue

	body, err := io.ReadAll(res.Body)

	if err != nil {
		c.Log.Error(err, "Failed to read response for list queues for vHost.", "vHost", vHost)
		return make([]RabbitQueue, 0), err
	}

	err = json.Unmarshal(body, &queues)

	if err != nil {
		c.Log.Error(err, "Failed to unmarshall response for list queues for vHost.", "vHost", vHost)
		return make([]RabbitQueue, 0), err
	}

	return queues, nil

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

func (c *RabbitClient) buildAMQPUrl(vHost string) string {
	host := os.Getenv("RABBIT_HOST")
	port := os.Getenv("RABBIT_PORT")

	return fmt.Sprintf("amqp://%s:%s@%s/%s", c.Http.Username, c.Http.Password, hhttp.BuildHost(host, port), vHost)
}

func BuildRabbitClient(log logr.Logger) *RabbitClient {
	host := os.Getenv("RABBIT_HOST")
	port := os.Getenv("RABBIT_CONSOLE_PORT")
	log.Info("Building Rabbit client with ", "host", host, "port", port)

	hclient := hhttp.BuildHttpClient(
		os.Getenv("RABBIT_ADMIN_USERNAME"),
		os.Getenv("RABBIT_ADMIN_PASSWORD"),
		hhttp.BuildHost(host, port),
	)

	c := RabbitClient{
		Log:  log,
		Http: hclient,
	}

	// check rabbit can be pinged
	_, err := c.Http.DoHttpRequest("api/overview", http.MethodGet, nil)

	if err != nil {
		return nil
	}

	return &c
}

func (c *RabbitClient) SetupRabbit(spec *hazyv1alpha1.HazyZoneSpec) error {
	c.Log.Info("Setting up Hazy Zone")

	// checking if VHost already exists.
	vhosts, err := c.listVHosts()

	if err != nil {
		c.Log.Error(err, "Failed to list vhosts.")
		return err
	}

	vHostAlreadyExists := func() bool {
		for _, vhost := range vhosts {
			if vhost.Name == spec.VHost {
				return true
			}
		}
		return false

	}()

	if !vHostAlreadyExists {
		// create vhost and user as not exists
		c.Log.Info("VHost ", spec.VHost, " does not exists, creating vhost/user")

		err = c.createAuthz(spec)
		if err != nil {
			c.Log.Error(err, "Failed to create user/vhosts.")
			return err
		}
	} else {
		c.Log.Info("VHost ", spec.VHost, " already exists, no need to create vhost/user")
	}

	curr_queues, err := c.listQueues(spec.VHost)

	if err != nil {
		c.Log.Error(err, "Failed to list queues.")
		return err
	}

	curr_queues_map := make(map[string]string)
	for _, curr_queue := range curr_queues {
		curr_queues_map[curr_queue.Name] = curr_queue.Name
	}
	wanted_queues_map := make(map[string]string)
	for _, wanted_queue := range spec.Queues {
		wanted_queues_map[wanted_queue] = wanted_queue
	}

	queuesToDelete := make([]string, 0)

	for k, _ := range curr_queues_map {
		_, ok := wanted_queues_map[k]
		if !ok {
			queuesToDelete = append(queuesToDelete, k)
		}
	}

	conn, err := amqp.Dial(c.buildAMQPUrl(spec.VHost))
	defer conn.Close()

	if err != nil {
		c.Log.Error(err, "Failed to connect to RabbitMQ")
		return err
	}

	ch, err := conn.Channel()
	defer ch.Close()
	if err != nil {
		c.Log.Error(err, "Failed to open a Channel  RabbitMQ")
		return err
	}

	// exchange, queuesToAdd
	c.Log.Info("Upserting Exchange for vHost", "vHost", spec.VHost)
	err = c.createExchange(spec, ch)
	if err != nil {
		c.Log.Error(err, "Failed to create exchange")
		return err
	}

	if len(queuesToDelete) > 0 {
		for _, queue := range queuesToDelete {
			res, err := c.deleteQueue(queue, spec.VHost)

			if err != nil {
				if res.StatusCode == http.StatusNotFound {
					c.Log.Info("Queue does not exist", "queue", queue)
				} else {
					c.Log.Error(err, "Failed to delete queue", "queue", queue)
					return err
				}

			}

		}
	}

	res_queues, err := c.listQueues(spec.VHost)
	if err != nil {
		c.Log.Error(err, "Failed to list queues for validation")
		return err
	}

	if len(res_queues) != len(wanted_queues_map) {
		if err != nil {
			c.Log.Error(err, "Failed to list queues for validation")
			return &SmokeQueueCountError{
				ExpectedQueues: len(wanted_queues_map), ActualQueues: len(res_queues),
			}
		}
	}

	c.Log.Info("Set up of Zone complete")
	return nil

}

func (c *RabbitClient) TearDownRabbit(spec *hazyv1alpha1.HazyZoneSpec) error {
	// tears down vhost, username (will delete exchange and

	// vhost, user
	err := c.deleteAuthz(spec)

	if err != nil {
		c.Log.Error(err, "Failed to delete the vhost/user", "vHost", spec.VHost)
		return err
	}
	return nil
}
