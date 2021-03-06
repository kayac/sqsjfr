package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/hashicorp/logutils"
	"github.com/kayac/sqsjfr"
)

var trapSignals = []os.Signal{
	syscall.SIGHUP,
	syscall.SIGINT,
	syscall.SIGTERM,
}
var sigCh = make(chan os.Signal, 1)

func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
}

func main() {
	err := _main()
	if err != nil {
		log.Println("[error]", err)
		os.Exit(1)
	}
}

func _main() error {
	var opt sqsjfr.Option
	var logLevel string

	flag.StringVar(&opt.QueueURL, "queue-url", "", "SQS queue URL")
	flag.StringVar(&opt.MessageTemplate, "message-template", "", "SQS message template(JSON)")
	flag.StringVar(&logLevel, "log-level", "info", "log level")
	flag.DurationVar(&opt.CheckInterval, "check-interval", time.Minute, "interval of checking for crontab modified")
	flag.BoolVar(&opt.DryRun, "dry-run", false, "dry run")
	flag.IntVar(&opt.StatsPort, "stats-port", sqsjfr.DefaultStatsServerPort, "stats HTTP server port")
	flag.VisitAll(envToFlag)
	flag.Parse()

	filter := &logutils.LevelFilter{
		Levels:   []logutils.LogLevel{"debug", "info", "warn", "error"},
		MinLevel: logutils.LogLevel(logLevel),
		Writer:   os.Stderr,
	}
	log.SetOutput(filter)

	args := flag.Args()
	if len(args) != 1 {
		return errors.New("crontab is required")
	}
	opt.CrontabURL = args[0]
	log.Printf("[debug] option:%#v", opt)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	signal.Notify(sigCh, trapSignals...)
	go func() {
		sig := <-sigCh
		log.Println("[info] got signal", sig)
		cancel()
	}()

	log.Println("[info] starting up")
	app, err := sqsjfr.New(ctx, &opt)
	if err != nil {
		return err
	}
	app.Run()
	return nil
}

func envToFlag(f *flag.Flag) {
	name := strings.ToUpper(strings.Replace(f.Name, "-", "_", -1))
	if s, ok := os.LookupEnv("SQSJFR_" + name); ok {
		f.Value.Set(s)
	}
}
