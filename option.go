package sqsjfr

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// Option represents sqsjfr option
type Option struct {
	Path            string
	QueueURL        string
	MessageTemplate string
}

// Validate validates option values.
func (opt *Option) Validate() error {
	if _, err := os.Stat(opt.Path); err != nil {
		return errors.Wrapf(err, "%s is not found", opt.Path)
	}

	region, accountID, queueName, err := parseQueueURL(opt.QueueURL)
	log.Println("[debug] region:", region)
	log.Println("[debug] accountID:", accountID)
	log.Println("[debug] queueName:", queueName)
	if err != nil {
		return err
	}
	if !strings.HasSuffix(queueName, ".fifo") {
		return errors.New("FIFO queue is required")
	}

	msg, err := newMessage(`echo "hello world!"`, opt.MessageTemplate, time.Now())
	if err != nil {
		return err
	}
	log.Println("[debug] generated message on validate", msg.String())

	return nil
}

// https://sqs.ap-northeast-1.amazonaws.com/123456789012/queue_name
func parseQueueURL(s string) (region string, accountID string, queueName string, err error) {
	u, err := url.Parse(s)
	if err != nil {
		return
	}
	h := strings.SplitN(u.Host, ".", 3)
	if u.Scheme != "https" || len(h) < 3 || h[0] != "sqs" || h[2] != "amazonaws.com" {
		err = fmt.Errorf("invalid queue URL:%s", s)
		return
	}
	region = h[1]

	p := strings.SplitN(u.Path, "/", 3)
	if len(p) != 3 {
		err = fmt.Errorf("invalid queue URL:%s", s)
	}
	accountID, queueName = p[1], p[2]
	return
}
