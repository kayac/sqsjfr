package sqsjfr

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/pkg/errors"
)

const DefaultStatsServerPort = 8061

// Option represents sqsjfr option
type Option struct {
	CrontabURL      string
	QueueURL        string
	MessageTemplate string
	CheckInterval   time.Duration
	DryRun          bool
	StatsPort       int

	sess *session.Session
}

// Validate validates option values.
func (opt *Option) Validate() error {
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

	msg, err := newMessage(
		`echo "hello world!"`,
		opt.MessageTemplate,
		time.Now(),
		Environments(map[string]string{}),
	)
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

func (app *App) ReadCrontabFile() (io.ReadCloser, error) {
	log.Println("[debug] crontab URL:", app.option.CrontabURL)
	u, err := url.Parse(app.option.CrontabURL)
	if err != nil {
		return nil, err
	}

	var src io.ReadCloser
	switch u.Scheme {
	case "s3":
		key := strings.TrimPrefix(u.Path, "/")
		src, err = readS3(app.sess, u.Host, key)
	case "http", "https":
		src, err = readHTTP(u.String())
	case "file", "":
		src, err = os.Open(u.Path)
	default:
		err = errors.Errorf("URL scheme %s is not supported", u.Scheme)
	}
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read from %s", u.String())
	}
	return src, nil
}

func readS3(sess *session.Session, bucket, key string) (io.ReadCloser, error) {
	svc := s3.New(sess)
	log.Printf("[debug] reading S3 bucket:%s key:%s", bucket, key)
	result, err := svc.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	return result.Body, nil
}

func readHTTP(u string) (io.ReadCloser, error) {
	resp, err := http.Get(u)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}
