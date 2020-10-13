package sqsjfr

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/hashicorp/go-envparse"
	"github.com/kayac/go-config"
	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
)

var (
	reSpace        = regexp.MustCompile("[ \t\n\v\f\r\u0085\u00A0]+")
	reTrimPrefix   = regexp.MustCompile("^[ \t\n\v\f\r\u0085\u00A0]+")
	reLooksLikeEnv = regexp.MustCompile("^[A-Za-z_][A-Za-z0-9_]*=")
)

const randomDelaySecond = 5

// SQSTimeout defines a timeout to send message.
var SQSTimeout = 10 * time.Second

func init() {
	rand.Seed(time.Now().UnixNano())
}

type message struct {
	Body      map[string]interface{}
	Command   string
	InvokedAt int64
	EntryID   int
	Env       map[string]string
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
	envs   map[string]string
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
	if err := app.load(); err != nil {
		return nil, err
	}
	return app, nil
}

// Run runs sqsjfr instance.
func (app *App) Run() {
	log.Println("[info] running daemon")
	app.cron.Start()
	<-app.ctx.Done()
	app.cron.Stop()
	log.Println("[info] shutting down")
	app.wg.Wait() // wait all invoke functions
	log.Println("[info] goodby")
}

func newMessage(command, messageTemplate string, now time.Time, envs map[string]string) (*message, error) {
	min := now.Truncate(time.Minute)
	msg := message{
		Body:      make(map[string]interface{}),
		Command:   command,
		InvokedAt: min.Unix(),
		Env:       envs,
	}
	loader := config.New()
	loader.Data = msg
	if err := loader.LoadWithEnvJSON(&msg.Body, messageTemplate); err != nil {
		return nil, fmt.Errorf("failed to create message with template: %s", err)
	}
	return &msg, nil
}

func readCrontab(r io.Reader, fn func(string) cron.Job) (*cron.Cron, map[string]string, error) {
	c := cron.New()
	scanner := bufio.NewScanner(r)
	lines := 0
	envsBuf := bytes.NewBuffer([]byte{})
	for scanner.Scan() {
		lines++
		line := scanner.Text()
		line = reTrimPrefix.ReplaceAllString(line, "")
		if line == "" || strings.HasPrefix(line, "#") { // skip
			envsBuf.WriteString("\n") // required for valid "error on line x"
			continue
		}
		if reLooksLikeEnv.MatchString(line) {
			envsBuf.WriteString(line + "\n")
			continue
		}
		envsBuf.WriteString("\n")
		f := reSpace.Split(line, 6)
		if len(f) < 6 {
			return nil, nil, fmt.Errorf("line %d, too few feilds > %s", lines, line)
		}
		spec := strings.Join(f[0:5], " ")
		command := f[5]
		job := fn(command)
		id, err := c.AddJob(spec, job)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "line %d, failed to add > %s", lines, line)
		}
		if j, ok := job.(*Job); ok {
			j.ID = id
		}
		log.Printf("[info] [entry:%d] registered > %s", id, line)
	}

	envs, err := envparse.Parse(envsBuf)
	if err != nil {
		return nil, nil, err
	}
	for name, value := range envs {
		log.Printf("[info] defined > %s=%s", name, value)
	}
	return c, envs, nil
}

func (app *App) load() error {
	log.Println("[info] loading crontab", app.option.Path)
	f, err := os.Open(app.option.Path)
	if err != nil {
		return err
	}

	app.cron, app.envs, err = readCrontab(f, app.newJob)
	if err != nil {
		return errors.Wrapf(err, "failed to read crontab %s", app.option.Path)
	}

	log.Printf("[info] %d entries registered", len(app.cron.Entries()))
	log.Printf("[info] %d environment variables defined", len(app.envs))
	return nil
}

func (app *App) send(msg *message) error {
	ctx, cancel := context.WithTimeout(context.Background(), SQSTimeout)
	defer cancel()
	in := &sqs.SendMessageInput{
		QueueUrl:               aws.String(app.option.QueueURL),
		MessageBody:            aws.String(msg.String()),
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
	delay := time.Second + time.Duration(rand.Int63n(randomDelaySecond*1000))*time.Millisecond
	log.Printf("[debug] new job command:%s delay:%s", command, delay)
	return &Job{
		Command: command,
		app:     app,
		delay:   delay,
	}
}

// Job represents a cron job.
type Job struct {
	ID      cron.EntryID
	Command string
	app     *App
	delay   time.Duration
}

// Run runs a Job.
func (j *Job) Run() {
	j.app.wg.Add(1)
	defer j.app.wg.Done()

	msg, err := newMessage(j.Command, j.app.option.MessageTemplate, time.Now(), j.app.envs)
	if err != nil {
		log.Printf("[warn] [entry:%d] %s", j.ID, err)
		return
	}
	time.Sleep(j.delay)
	log.Printf("[info] [entry:%d] invoke job %s", j.ID, msg.String())
	if err := j.app.send(msg); err != nil {
		log.Printf("[error] [entry:%d] failed to send message: %s", j.ID, err)
	}
}
