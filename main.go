package main

import (
	"fmt"
	"io"
	"os"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func main() {
	os.Exit(run(os.Args, os.Stdout, os.Stderr))
}

func msg(err error, errStream io.Writer) int {
	if err != nil {
		fmt.Fprintf(errStream, "%s: %v\n", os.Args[0], err)
		return 1
	}
	return 0
}

func run(args []string, outStream, errStream io.Writer) int {
	app := cli.NewApp()
	app.Name = "ci-modules-update"
	app.Usage = "create a modules update PR"
	app.Version = "0.1.3"
	app.Flags = commandFlags()
	app.Action = appRun

	return msg(app.Run(args), outStream)
}

func appRun(c *cli.Context) error {
	var err error

	if err = checkRequiredArguments(c); err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	updater := NewModulesUpdater(c)
	if err = updater.Run(); err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	return nil
}

func commandFlags() []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:   "github_access_token",
			Value:  "",
			Usage:  "GitHub access token",
			EnvVar: "GITHUB_ACCESS_TOKEN",
		},
		cli.StringFlag{
			Name:   "user, u",
			Value:  "",
			Usage:  "Git user name",
			EnvVar: "GIT_USER_NAME",
		},
		cli.StringFlag{
			Name:   "email, e",
			Value:  "",
			Usage:  "Git user email",
			EnvVar: "GIT_USER_EMAIL",
		},
		cli.StringFlag{
			Name:   "repository, r",
			Value:  "",
			Usage:  "Repository url",
			EnvVar: "REPOSITORY_URL",
		},
		cli.BoolFlag{
			Name:  "dry-run",
			Usage: "only show diff",
		},
	}
}

func checkRequiredArguments(c *cli.Context) error {
	if c.Bool("dry-run") {
		return nil
	}

	if c.String("user") == "" {
		return errors.New("please set Git user name")
	}
	if c.String("repository") == "" {
		return errors.New("please set repository URL")
	}
	if c.String("github_access_token") == "" {
		return errors.New("please set GitHub access token")
	}

	return nil
}
