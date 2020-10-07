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

	"github.com/kayac/sqsjfr"
)

func main() {
	err := _main()
	if err != nil {
		log.Println("[error]", err)
		os.Exit(1)
	}
}

func _main() error {
	var opt sqsjfr.Option

	flag.StringVar(&opt.QueueURL, "queue-url", "", "SQS queue URL")
	flag.StringVar(&opt.MessageTemplate, "message-template", "", "SQS message template(JSON)")
	flag.VisitAll(envToFlag)
	flag.Parse()
	args := flag.Args()
	if len(args) != 1 {
		return errors.New("crotab file path is required")
	}
	opt.Path = args[0]
	log.Printf("[debug] option:%#v", opt)
	if err := opt.Validate(); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	sigs := []os.Signal{
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
	}
	ch := make(chan os.Signal, 10)
	signal.Notify(ch, sigs...)
	go func() {
		sig := <-ch
		log.Println("[info] got signal", sig)
		cancel()
	}()

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
