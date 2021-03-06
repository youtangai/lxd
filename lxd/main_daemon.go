package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"

	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"

	"github.com/lxc/lxd/lxd/sys"
	"github.com/lxc/lxd/shared/logger"
)

type cmdDaemon struct {
	global *cmdGlobal

	// Common options
	flagGroup string
}

func (c *cmdDaemon) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "lxd"
	cmd.Short = "The LXD container manager (daemon)"
	cmd.Long = `Description:
  The LXD container manager (daemon)

  This is the LXD daemon command line. It's typically started directly by your
  init system and interacted with through a tool like ` + "`lxc`" + `.

  There are however a number of subcommands that let you interact directly with
  the local LXD daemon and which may not be performed through the REST API alone.
`
	cmd.RunE = c.Run
	cmd.Flags().StringVar(&c.flagGroup, "group", "", "The group of users that will be allowed to talk to LXD"+"``")

	return cmd
}

func (c *cmdDaemon) Run(cmd *cobra.Command, args []string) error {
	if len(args) > 1 || (len(args) == 1 && args[0] != "daemon" && args[0] != "") {
		return fmt.Errorf("unknown command \"%s\" for \"%s\"", args[0], cmd.CommandPath())
	}

	// Only root should run this
	if os.Geteuid() != 0 {
		return fmt.Errorf("This must be run as root")
	}

	neededPrograms := []string{"setfacl", "rsync", "tar", "unsquashfs", "xz"}
	for _, p := range neededPrograms {
		_, err := exec.LookPath(p)
		if err != nil {
			return err
		}
	}

	conf := DefaultDaemonConfig()
	conf.Group = c.flagGroup
	conf.Trace = c.global.flagLogTrace
	d := NewDaemon(conf, sys.DefaultOS())

	err := d.Init()
	if err != nil {
		return err
	}

	ch := make(chan os.Signal)
	signal.Notify(ch, unix.SIGPWR)
	signal.Notify(ch, unix.SIGINT)
	signal.Notify(ch, unix.SIGQUIT)
	signal.Notify(ch, unix.SIGTERM)

	s := d.State()
	select {
	case sig := <-ch:
		if sig == unix.SIGPWR {
			logger.Infof("Received '%s signal', shutting down containers", sig)
			containersShutdown(s)
			networkShutdown(s)
		} else {
			logger.Infof("Received '%s signal', exiting", sig)
		}

	case <-d.shutdownChan:
		logger.Infof("Asked to shutdown by API, shutting down containers")
		d.Kill()
		containersShutdown(s)
		networkShutdown(s)
	}

	return d.Stop()
}
