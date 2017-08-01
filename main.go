package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	log "github.com/Sirupsen/logrus"
	"github.com/jessevdk/go-flags"
)

type Options struct {
	Configuration string `short:"c" long:"configuration" description:"the configuration file" default:"supervisord.conf"`
}

func init() {
	log.SetOutput(os.Stdout)
	log.SetLevel(log.DebugLevel)
}

func initSignals(s *Supervisor) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		log.WithFields(log.Fields{"signal": sig}).Info("receive a signal to stop all process & exit")
		s.procMgr.ForEachProcess(func(proc *Process) {
			proc.Stop(true)
		})
		os.Exit(-1)
	}()

}

var options Options
var parser = flags.NewParser(&options, flags.Default & ^flags.PrintErrors)

func main() {
	if _, err := parser.Parse(); err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		}
	}
	s := NewSupervisor(options.Configuration)
	initSignals(s)
	if err := s.Reload(); err != nil {
		panic(err)
	}
}
