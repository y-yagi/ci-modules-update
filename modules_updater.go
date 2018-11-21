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
		fmt.Print("all modules are already up to date.\n")
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
	changedLabel := "**Changed:**\n\n"
	addedLabel := "**Added:**\n\n"
	changed := changedLabel
	added := addedLabel

	var result string
	var existInBefore bool

	for _, afterRequire := range afterMod.Require {
		for _, beforeRequire := range beforeMod.Require {
			if beforeRequire.Path == afterRequire.Path {
				if beforeRequire.Version != afterRequire.Version {
					changed += updater.generateDiffLink(&beforeRequire, &afterRequire)
				}
				existInBefore = true
				break
			}
		}
		if !existInBefore {
			added += fmt.Sprintf("* [%s](%s)\n", afterRequire.Path, updater.generateRepoURL(&afterRequire))
		}
		existInBefore = false
	}

	if added != addedLabel {
		result += added + "\n\n"
	}
	if changed != changedLabel {
		result += changed
	}
	return result
}

func (updater *ModulesUpdater) generateDiffLink(before *Require, after *Require) string {
	path := after.Path
	prev := updater.cutTagFromVersion(before.Version)
	cur := updater.cutTagFromVersion(after.Version)
	url := updater.generateRepoURL(after)

	if strings.Contains(url, "github.com") {
		return fmt.Sprintf("* [%s](%s) [%s...%s](%s/compare/%s...%s)\n", path, url, prev, cur, url, prev, cur)
	}
	return fmt.Sprintf("* [%s](%s) %s...%s\n", path, path, prev, cur)
}

func (updater *ModulesUpdater) generateRepoURL(require *Require) string {
	path := require.Path
	repos := map[string]string{
		"golang.org/x/":               "https://github.com/golang/",
		"cloud.google.com/go":         "https://github.com/GoogleCloudPlatform/google-cloud-go",
		"google.golang.org/api":       "https://github.com/googleapis/google-api-go-client",
		"google.golang.org/appengine": "https://github.com/golang/appengine",
	}

	for pattern, repo := range repos {
		pattenLen := len(pattern)
		if (len(path) >= pattenLen) && (path[:pattenLen] == pattern) {
			if pattern == "golang.org/x/" {
				return repo + path[pattenLen:]
			}
			return repo
		}
	}

	return "https://" + path
}

func (updater *ModulesUpdater) cutTagFromVersion(v string) string {
	v = strings.TrimSuffix(v, "+incompatible")
	if strings.HasPrefix(v, "v0.0.0-") {
		// NOTE: "pseudo-version" is `v0.0.0-yyyymmddhhmmss-abcdefabcdef` format
		v = strings.Split(v, "-")[2]
	}
	return v
}
