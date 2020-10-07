package sqsjfr

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/kayac/go-config"
	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
)

var reSpace = regexp.MustCompile("[ \t\n\v\f\r\u0085\u00A0]+")
var reTrimPrefix = regexp.MustCompile("^[ \t\n\v\f\r\u0085\u00A0]+")

// SQSTimeout defines a timeout to send message.
var SQSTimeout = 10 * time.Second

type message struct {
	Body      map[string]interface{}
	Command   string
	InvokedAt int64
	EntryID   int
}

func (m message) String() string {
	var b strings.Builder
	json.NewEncoder(&b).Encode(m.Body)
	return strings.TrimSuffix(b.String(), "\n")
}

func (m message) DeduplicationID() string {
	h := sha256.New()
	h.Write([]byte(m.String()))
	h.Write([]byte(strconv.FormatInt(m.InvokedAt, 10)))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// App represents a sqsjfr application instance.
type App struct {
	option *Option
	cron   *cron.Cron
	sqs    *sqs.SQS
	ctx    context.Context
	wg     sync.WaitGroup
}

// New creates an App instance.
func New(ctx context.Context, opt *Option) (*App, error) {
	sess, err := session.NewSession()
	if err != nil {
		return nil, err
	}
	app := &App{
		option: opt,
		sqs:    sqs.New(sess),
		ctx:    ctx,
	}
	if err := app.load(app.newJob); err != nil {
		return nil, err
	}
	return app, nil
}

// Run runs sqsjfr instance.
func (app *App) Run() {
	app.cron.Start()
	<-app.ctx.Done()
	app.cron.Stop()
	log.Println("[info] shutting down")
	app.wg.Wait() // wait all invoke functions
	log.Println("[info] goodby")
}

func newMessage(command, messageTemplate string) (*message, error) {
	now := time.Now().Truncate(time.Minute)
	msg := message{
		Body:      make(map[string]interface{}),
		Command:   command,
		InvokedAt: now.Unix(),
	}
	loader := config.New()
	loader.Data = msg
	if err := loader.LoadWithEnvJSON(&msg.Body, messageTemplate); err != nil {
		return nil, fmt.Errorf("failed to create message with template: %s", messageTemplate)
	}
	return &msg, nil
}

func readCrontab(r io.Reader, fn func(string) cron.Job) (*cron.Cron, error) {
	c := cron.New()
	scanner := bufio.NewScanner(r)
	lines := 0
	for scanner.Scan() {
		lines++
		line := scanner.Text()
		line = reTrimPrefix.ReplaceAllString(line, "")
		if line == "" || strings.HasPrefix(line, "#") { // skip
			continue
		}
		f := reSpace.Split(line, 6)
		if len(f) < 6 {
			return nil, fmt.Errorf("line:%d too few feilds > %s", lines, line)
		}
		spec := strings.Join(f[0:5], " ")
		command := f[5]
		job := fn(command)
		id, err := c.AddJob(spec, job)
		if err != nil {
			return nil, errors.Wrapf(err, "line:%d failed to add > %s", lines, line)
		}
		if j, ok := job.(*Job); ok {
			j.ID = id
		}
		log.Printf("[info] [entry:%d] registered > %s", id, line)
	}
	return c, nil
}

func (app *App) load(fn func(string) cron.Job) error {
	f, err := os.Open(app.option.Path)
	if err != nil {
		return err
	}

	app.cron, err = readCrontab(f, app.newJob)
	if err != nil {
		return errors.Wrapf(err, "failed to read crontab %s", app.option.Path)
	}
	return nil
}

func (app *App) send(msg *message) error {
	ctx, cancel := context.WithTimeout(context.Background(), SQSTimeout)
	defer cancel()
	in := &sqs.SendMessageInput{
		MessageBody:            aws.String(msg.String()),
		QueueUrl:               aws.String(app.option.QueueURL),
		MessageDeduplicationId: aws.String(msg.DeduplicationID()),
		MessageGroupId:         aws.String("sqsjfr"),
	}
	log.Println("[debug] sending message:", in.String())
	out, err := app.sqs.SendMessageWithContext(ctx, in)
	if err != nil {
		return err
	}
	log.Println("[debug] sent messageID:", *out.MessageId)
	return nil
}

func (app *App) newJob(command string) cron.Job {
	return &Job{
		Command: command,
		app:     app,
	}
}

// Job represents a cron job.
type Job struct {
	ID      cron.EntryID
	Command string
	app     *App
}

// Run runs a Job.
func (j *Job) Run() {
	j.app.wg.Add(1)
	defer j.app.wg.Done()
	msg, err := newMessage(j.Command, j.app.option.MessageTemplate)
	if err != nil {
		log.Printf("[warn] [entry:%d] %s", j.ID, err)
		return
	}
	log.Printf("[info] [entry:%d] invoke job %s", j.ID, msg.String())
	if err := j.app.send(msg); err != nil {
		log.Printf("[error] [entry:%d] failed to send message: %s", j.ID, err)
	}
}
