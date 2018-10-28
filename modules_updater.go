package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"golang.org/x/oauth2"
)

// ModulesUpdater is a struct fo update.
type ModulesUpdater struct {
	cli    *cli.Context
	dryRun bool
}

// GoMod is a struct fo go.mod.
type GoMod struct {
	Module  Module
	Require []Require
	Exclude []Module
	Replace []Replace
}

// Module is a struct fo package.
type Module struct {
	Path    string
	Version string
}

// Require is a struct fo require package.
type Require struct {
	Path     string
	Version  string
	Indirect bool
}

// Replace is a struct fo replace package.
type Replace struct {
	Old Module
	New Module
}

// NewModulesUpdater creates a new updater.
func NewModulesUpdater(cli *cli.Context) *ModulesUpdater {
	updater := &ModulesUpdater{cli: cli}

	return updater
}

// Run update.
func (updater *ModulesUpdater) Run() error {
	var result bool

	updater.dryRun = updater.cli.Bool("dry-run")

	beforeMod, err := updater.readModules(".")
	if err != nil {
		return err
	}

	if err = updater.runModuleUpdate(); err != nil {
		return err
	}

	if result, err = updater.isNeedUpdate(); err != nil {
		return err
	}

	if !result {
		return nil
	}

	afterMod, err := updater.readModules(".")
	if err != nil {
		return err
	}

	ctx := context.Background()
	token := updater.cli.String("github_access_token")
	client := updater.gitHubClient(token, &ctx)

	user := updater.cli.String("user")
	repo := updater.cli.String("repository")
	email := updater.cli.String("email")
	if len(email) == 0 {
		email = user + "@users.noreply.github.com"
	}

	branch := "modules-update-" + time.Now().Format("2006-01-02-150405")

	updater.createBranchAndCommit(user, email, token, repo, branch)

	return updater.createPullRequest(&ctx, client, beforeMod, afterMod, repo, branch)
}

func (updater *ModulesUpdater) readModules(dir string) (*GoMod, error) {
	file := filepath.Join(dir, "go.mod")
	cmd := exec.Command("go", "mod", "edit", "-json", file)
	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	stdoutStederr, err := cmd.CombinedOutput()
	if err != nil {
		return nil, errors.New("run `go mod edit` failed. cause: " + string(stdoutStederr))
	}

	var gomod GoMod
	if err := json.Unmarshal([]byte(stdoutStederr), &gomod); err != nil {
		return nil, err
	}

	return &gomod, nil
}

func (updater *ModulesUpdater) runModuleUpdate() error {
	cmd := exec.Command("go", "get", "-u")
	cmd.Env = append(os.Environ(), "GO111MODULE=on")

	if stdoutStederr, err := cmd.CombinedOutput(); err != nil {
		return errors.New("run `go get -u` failed. cause: " + string(stdoutStederr))
	}
	return nil
}

func (updater *ModulesUpdater) isNeedUpdate() (bool, error) {
	stdoutStederr, err := exec.Command("git", "diff", "--name-only").CombinedOutput()
	if err != nil {
		return false, errors.New("run `git diff` failed. cause: " + string(stdoutStederr))
	}

	result := strings.Contains(string(stdoutStederr), "go.mod")
	return result, nil
}

func (updater *ModulesUpdater) createBranchAndCommit(username, useremail, token, repo, branch string) {
	if updater.dryRun {
		exec.Command("git", "checkout", "go.mod", "go.sum").Run()
		return
	}

	remote := "https://" + token + "@github.com/" + repo
	exec.Command("git", "remote", "add", "github-url-with-token", remote).Run()
	exec.Command("git", "config", "user.name", username).Run()
	exec.Command("git", "config", "user.email", useremail).Run()
	exec.Command("git", "add", "go.mod", "go.sum").Run()
	exec.Command("git", "commit", "-m", "Run 'go get -u'").Run()
	exec.Command("git", "branch", "-M", branch).Run()
	exec.Command("git", "push", "-q", "github-url-with-token", branch).Run()
}

func (updater *ModulesUpdater) gitHubClient(accessToken string, ctx *context.Context) *github.Client {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: accessToken},
	)
	tc := oauth2.NewClient(*ctx, ts)
	return github.NewClient(tc)
}

func (updater *ModulesUpdater) createPullRequest(ctx *context.Context, client *github.Client, beforeMod *GoMod, afterMod *GoMod, repo, branch string) error {
	title := github.String("modules update at " + time.Now().Format("2006-01-02 15:04:05"))
	body := github.String(updater.generatePullRequestBody(beforeMod, afterMod))
	base := github.String("master")
	ownerAndRepo := strings.Split(repo, "/")
	head := github.String(ownerAndRepo[0] + ":" + branch)
	pr := &github.NewPullRequest{Title: title, Head: head, Base: base, Body: body}

	if updater.dryRun {
		fmt.Printf("\n%v\n", *body)
		return nil
	}

	_, _, err := client.PullRequests.Create(*ctx, ownerAndRepo[0], ownerAndRepo[1], pr)
	return err
}

func (updater *ModulesUpdater) generatePullRequestBody(beforeMod *GoMod, afterMod *GoMod) string {
	result := "**Changed:**\n\n"

	for _, afterRequire := range afterMod.Require {
		for _, beforeRequire := range beforeMod.Require {
			if beforeRequire.Path == afterRequire.Path {
				if beforeRequire.Version != afterRequire.Version {
					result += updater.generateDiffLink(&beforeRequire, &afterRequire)
				}
				break
			}
		}
	}

	return result
}

func (updater *ModulesUpdater) generateDiffLink(before *Require, after *Require) string {
	var compareLink string
	var pkg string
	var url string

	name := after.Path
	golangOrg := "golang.org/x/"
	golangOrgLen := len(golangOrg)
	cloudGoogleCom := "cloud.google.com/go"
	cloudGoogleComLen := len(cloudGoogleCom)
	googleGolangOrg := "google.golang.org/api"
	googleGolangOrgLen := len(googleGolangOrg)

	prev := updater.generateTagFromVersion(before.Version)
	cur := updater.generateTagFromVersion(after.Version)

	if strings.Contains(name, "github.com") {
		compareLink = fmt.Sprintf("[%s...%s](https://%s/compare/%s...%s)", prev, cur, name, prev, cur)
		return fmt.Sprintf("* [%s](https://%s) %s\n", name, name, compareLink)
	} else if name[:golangOrgLen] == golangOrg {
		pkg = name[golangOrgLen:]
		url = "https://github.com/golang/" + pkg
		return fmt.Sprintf("* [%s](%s) [%s...%s](%s/compare/%s...%s)\n", name, url, prev, cur, url, prev, cur)
	} else if name[:cloudGoogleComLen] == cloudGoogleCom {
		url = "https://github.com/GoogleCloudPlatform/google-cloud-go"
		return fmt.Sprintf("* [%s](%s) [%s...%s](%s/compare/%s...%s)\n", name, url, prev, cur, url, prev, cur)
	} else if name[:googleGolangOrgLen] == googleGolangOrg {
		url = "https://github.com/googleapis/google-api-go-client"
		return fmt.Sprintf("* [%s](%s) [%s...%s](%s/compare/%s...%s)\n", name, url, prev, cur, url, prev, cur)
	}
	return fmt.Sprintf("* [%s](https://%s) %s...%s\n", name, name, prev, cur)
}

func (updater *ModulesUpdater) generateTagFromVersion(v string) string {
	v = strings.TrimSuffix(v, "+incompatible")
	if strings.HasPrefix(v, "v0.0.0-") {
		// NOTE: "pseudo-version" is `v0.0.0-yyyymmddhhmmss-abcdefabcdef` format
		v = strings.Split(v, "-")[2]
	}
	return v
}
