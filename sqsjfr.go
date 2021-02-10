package sqsjfr

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"math/rand"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/hashicorp/go-envparse"
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

type errorReload struct{}

func (e errorReload) Error() string {
	return "reloading required"
}

var errReload = errorReload{}

type logger interface {
	Println(...interface{})
	Printf(string, ...interface{})
}

type nullLogger struct{}

func (l nullLogger) Println(v ...interface{}) {}

func (l nullLogger) Printf(format string, v ...interface{}) {}

func init() {
	rand.Seed(time.Now().UnixNano())
}

// App represents a sqsjfr application instance.
type App struct {
	option *Option
	cron   *cron.Cron
	envs   Environments
	sqs    *sqs.SQS
	sess   *session.Session

	ctx    context.Context
	reload context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	digest []byte

	stats *Stats
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
		sess:   sess,
		ctx:    ctx,
		stats:  &Stats{},
	}
	return app, opt.Validate()
}

// Run runs sqsjfr instance.
func (app *App) Run() error {
	go func() {
		if err := app.runStatsServer(); err != nil {
			panic(err)
		}
	}()
	for {
		app.reload, app.cancel = context.WithCancel(context.Background())
		if err := app.run(); err == nil {
			// normarly shutdown
			log.Println("[info] goodby")
			return nil
		} else if _, ok := err.(errorReload); ok {
			log.Println("[info] reloading")
			continue
		} else {
			return err
		}
	}
}

func (app *App) watch() {
	interval := app.option.CheckInterval
	if interval == 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	log.Printf("[info] starting up crontab watcher interval %s", interval)
	for {
		select {
		case <-app.ctx.Done():
			// canceled by others
			return
		case <-ticker.C:
		}
		newDigest, err := func() ([]byte, error) {
			f, err := app.ReadCrontabFile()
			if err != nil {
				return nil, err
			}
			defer f.Close()
			h := sha256.New()
			r := io.TeeReader(f, h)
			_, _, err = readCrontab(r, newDummyJob)
			return h.Sum(nil), err
		}()
		if err != nil {
			log.Println("[warn]", err)
			continue
		}
		if !bytes.Equal(app.digest, newDigest) {
			log.Printf("[info] crontab is modified %x -> %x", app.digest, newDigest)
			app.cancel()
			return
		}
		log.Printf("[debug] digest unchanged %x", app.digest)
	}
}

func (app *App) run() error {
	if err := app.load(); err != nil {
		return err
	}
	if app.option.DryRun {
		log.Println("[info] dry run OK")
		return nil
	}

	go app.watch()

	log.Println("[info] running daemon")
	app.cron.Start()
	var err error
	select {
	case <-app.ctx.Done():
	case <-app.reload.Done():
		err = errReload
	}
	app.cron.Stop()
	log.Println("[info] shutting down")
	app.wg.Wait() // wait all invoke functions
	return err
}

func readCrontab(r io.Reader, fn func(string) cron.Job) (*cron.Cron, Environments, error) {
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
			log.Printf("[info] [entry:%d] registered > %s", id, line)
		}
	}

	envs, err := envparse.Parse(envsBuf)
	if err != nil {
		return nil, nil, err
	}
	return c, Environments(envs), nil
}

func (app *App) load() error {
	log.Println("[info] loading crontab", app.option.CrontabURL)
	f, err := app.ReadCrontabFile()
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	r := io.TeeReader(f, h)
	app.cron, app.envs, err = readCrontab(r, app.newJob)
	if err != nil {
		return errors.Wrapf(err, "failed to read crontab %s", app.option.CrontabURL)
	}
	app.digest = h.Sum(nil)

	log.Printf("[debug] crontab digest %x", app.digest)
	log.Printf("[info] %d entries registered", len(app.cron.Entries()))
	log.Printf("[info] %d environment variables defined", len(app.envs))
	atomic.StoreInt64(&app.stats.Entries.Registered, int64(len(app.cron.Entries())))
	for name, value := range app.envs {
		log.Printf("[info] export %s=%s", name, value)
	}
	return nil
}

func (app *App) send(msg *Message) error {
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
	log.Printf("[debug] new job command:%s", command)
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

func (j *Job) delay() time.Duration {
	return 100 * time.Millisecond * time.Duration(j.ID)
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
	time.Sleep(j.delay())
	log.Printf("[info] [entry:%d] invoke job %s", j.ID, msg.String())
	if err := j.app.send(msg); err != nil {
		atomic.AddInt64(&j.app.stats.Invocations.Failed, 1)
		log.Printf("[error] [entry:%d] failed to send message: %s", j.ID, err)
	} else {
		atomic.AddInt64(&j.app.stats.Invocations.Succeeded, 1)
	}
}

type dummyJob struct{}

func newDummyJob(string) cron.Job {
	return &dummyJob{}
}

func (j *dummyJob) Run() {}
