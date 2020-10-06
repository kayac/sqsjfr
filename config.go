package sqsjfr

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/kayac/sqsjkr"
)

type Config struct {
	Path            string             `yaml:"path"`
	QueueURL        string             `yaml:"queue_url"`
	MessageTemplate sqsjkr.MessageBody `yaml:"message_template"`
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
