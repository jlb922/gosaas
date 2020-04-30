package queue

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-redis/redis"
	"github.com/robfig/cron"
)

var (
	client    *redis.Client
	pubsub    *redis.PubSub
	scheduler *cron.Cron
	isDev     bool

	emailer *Email
	biller  *Billing

	executors map[TaskID]TaskExecutor
)

// New initializes the queue tasks; intially called from cache package
func New(rc *redis.Client, isDev bool, ex map[TaskID]TaskExecutor) {
	client = rc

	// built-in executor
	emailer = &Email{}
	biller = &Billing{}

	// We are adding the emailer variable here and based on the environment flag
	// we receive from the cache package we assign the correct implementation to our
	// Send value.
	if isDev {
		emailer.Send = emailer.sendEmailDev
	} else {
		emailer.Send = emailer.sendEmailProd
	}

	executors = ex
}

// SetAsSubscriber makes this instance a Pub/Sub subscriber. Each message queued
// will be processed by this instance. Creates a subscriber to channel "q"
func SetAsSubscriber() {
	scheduler = cron.New()

	pubsub = client.Subscribe("q")
	if err := pubsub.Ping("test"); err != nil {
		log.Fatal("unable to ping pubsub", err)
	}
	defer func() {
		pubsub.Close()
		scheduler.Stop()
	}()

	if _, err := pubsub.Receive(); err != nil {
		log.Fatal("unable to receive from pubsub channel", err)
	}

	// we initialize our scheduler (cron)
	go setupCron()

	ch := pubsub.Channel()

	for {
		msg, ok := <-ch
		if !ok {
			log.Fatal("redis pub/sub is down")
			break
		}
		// process function in its own go routine to improve the speed our queue subscriber can dequeue the task
		go process(msg)
	}
}

// start the Cron executor
// first try to load the file and parse its content. For each line in the file, we create
//  the function that will be executed when the correct time is reached.
func setupCron() {
	if _, err := os.Stat("tasks.cron"); os.IsNotExist(err) {
		log.Println("no tasks.cron file found, skipping scheduler setup")
		return
	}

	b, err := ioutil.ReadFile("tasks.cron")
	if err != nil {
		log.Println("error while reading tasks.cron", err)
		return
	}

	lines := strings.Split(string(b), "\n")
	if len(lines) == 0 {
		log.Println("no tasks found in tasks.cron, skipping scheduler setup")
		return
	}

	for _, line := range lines {
		exp, url := parseTask(line)

		// line below has been problematic
		err := scheduler.AddFunc(exp, func() {
			req, err := http.NewRequest("POST", url, bytes.NewReader(b))
			if err != nil {
				log.Println("error while creating an HTTP request to", url)
				return
			}

			req.SetBasicAuth("todo", "here")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				log.Println("error while executing an HTTP request to", url)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode >= 400 {
				log.Println("scheduler HTTP request to ", url, "failed with HTTP status", resp.StatusCode)
			}
		})

		if err != nil {
			log.Fatal("unable to create cron tasks", err)
		}
	}

	scheduler.Start()
}

func parseTask(s string) (exp string, url string) {
	tokens := strings.Split(s, " ")
	url = strings.Join(tokens[len(tokens)-1:], " ")
	exp = strings.Join(tokens[0:len(tokens)-1], " ")
	return
}

// Enqueue adds a task to the queue.
func Enqueue(id TaskID, data interface{}) error {
	qt := QueueTask{
		ID:      id,
		Data:    data,
		Created: time.Now(),
	}
	fmt.Println("Enqueing:", data)
	b, err := json.Marshal(qt)
	if err != nil {
		return err
	}
	return client.Publish("q", string(b)).Err()
}

// process function which is called everytime a new task is queued.
func process(msg *redis.Message) {
	var qt QueueTask
	// deserialize the message payload into a QueueTask and we select the right executor based on the ID.
	if err := json.Unmarshal([]byte(msg.Payload), &qt); err != nil {
		log.Fatal("unable to decode this Redis message", err)
	}

	var exec TaskExecutor

	switch qt.ID {
	case TaskEmail:
		exec = emailer
	case TaskCreateInvoice:
		exec = biller
	default:
		if ex, ok := executors[qt.ID]; ok {
			exec = ex
		}
	}

	// call the Run function and log the error if one occurs.
	if err := exec.Run(qt); err != nil {
		//TODO: better to log those critical errors
		log.Println("error while executing this task", qt.ID, err)
	}
	fmt.Println("Call successfully made")
}
