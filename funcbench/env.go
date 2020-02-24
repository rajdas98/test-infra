// Copyright 2020 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package main

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
)

type Environment interface {
	BenchFunc() string
	CompareTarget() string
	IsRaceEnabled() bool

	PostErr(err string) error
	PostResults(cmps []BenchCmp) error

	Repo() *git.Repository
}

type environment struct {
	logger Logger

	benchFunc     string
	compareTarget string
	isRaceEnabled bool

	home string
}

func (e environment) BenchFunc() string     { return e.benchFunc }
func (e environment) CompareTarget() string { return e.compareTarget }
func (e environment) IsRaceEnabled() bool   { return e.isRaceEnabled }

type Local struct {
	environment

	repo *git.Repository
}

func newLocalEnv(e environment) (Environment, error) {
	r, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, err
	}

	return &Local{environment: e, repo: r}, nil
}

func (l *Local) PostErr(string) error { return nil } // Noop. We will see error anyway.

// formatNs formats ns measurements to expose a useful amount of
// precision. It mirrors the ns precision logic of testing.B.
func formatNs(ns float64) string {
	prec := 0
	switch {
	case ns < 10:
		prec = 2
	case ns < 100:
		prec = 1
	}
	return strconv.FormatFloat(ns, 'f', prec, 64)
}

func (l *Local) PostResults(cmps []BenchCmp) error {
	fmt.Println("Results:")
	Render(os.Stdout, cmps, false, false, l.compareTarget)
	return nil
}

func (l *Local) Repo() *git.Repository { return l.repo }

// TODO: Add unit test(!).
type GitHub struct {
	environment

	repo    *git.Repository
	client  *gitHubClient
	logLink string
}

func newGitHubEnv(ctx context.Context, logger Logger, eventFilePath string) (Environment, error) {
	data, err := ioutil.ReadFile(eventFilePath)
	if err != nil {
		return nil, err
	}

	event, err := github.ParseWebHook("issue_comment", data)
	if err != nil {
		return nil, err
	}

	if err := os.Chdir(os.Getenv("GITHUB_WORKSPACE")); err != nil {
		return nil, err
	}

	issue, ok := event.(*github.IssueCommentEvent)
	if !ok {
		return nil, errors.New("only issue_comment event is supported")
	}

	r, err := git.PlainCloneContext(ctx, *issue.GetRepo().Name, false, &git.CloneOptions{
		URL:      fmt.Sprintf("https://github.com/%s/%s.git", *issue.GetRepo().Owner.Login, *issue.GetRepo().Name),
		Progress: os.Stdout,
	})
	if err != nil {
		// If repo already exists, git.ErrRepositoryAlreadyExists will be returned.
		return nil, errors.Wrap(err, "git clone")
	}

	ghClient := newGitHubClient(issue)

	// TODO: Explain Where those files come from?
	benchFunc, err := ioutil.ReadFile("/github/home/commentMonitor/REGEX")
	if err != nil {
		return nil, err
	}
	raceArgument, err := ioutil.ReadFile("/github/home/commentMonitor/RACE")
	if err != nil {
		return nil, err
	}
	compareTarget, err := ioutil.ReadFile("/github/home/commentMonitor/BRANCH")
	if err != nil {
		return nil, err
	}

	if err := os.Chdir(filepath.Join(os.Getenv("GITHUB_WORKSPACE"), ghClient.repo)); err != nil {
		return nil, errors.Wrap(err, "changing to GITHUB_WORKSPACE dir")
	}

	g := &GitHub{
		environment: environment{
			logger:        logger,
			benchFunc:     string(benchFunc),
			compareTarget: string(compareTarget),
			isRaceEnabled: string(raceArgument) != "-no-race",
			home:          os.Getenv("HOME"),
		},
		repo:    r,
		client:  ghClient,
		logLink: fmt.Sprintf("Full logs at: https://github.com/%s/%s/commit/%s/checks", ghClient.owner, ghClient.repo, ghClient.latestCommitHash),
	}

	if err := os.Setenv("GO111MODULE", "on"); err != nil {
		return nil, err
	}
	if err := os.Setenv("CGO_ENABLED", "0"); err != nil {
		return nil, err
	}

	wt, err := g.repo.Worktree()
	if err != nil {
		return nil, err
	}

	if err := r.FetchContext(ctx, &git.FetchOptions{}); err != nil && err != git.NoErrAlreadyUpToDate {
		if pErr := g.PostErr("Switch (fetch) to a pull request branch failed"); pErr != nil {
			return nil, errors.Wrapf(err, "posting a comment for `checkout` command execution error; postComment err:%v", pErr)
		}
		return nil, err
	}

	if err = wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.ReferenceName(fmt.Sprintf("pull/%d/head:pullrequest", *issue.GetIssue().Number)),
	}); err != nil {
		if pErr := g.PostErr("Switch to a pull request branch failed"); pErr != nil {
			return nil, errors.Wrapf(err, "posting a comment for `checkout` command execution error; postComment err:%v", pErr)
		}
		return nil, err
	}
	return g, nil
}

func (g *GitHub) Repo() *git.Repository { return g.repo }

type gitHubClient struct {
	owner            string
	repo             string
	latestCommitHash string
	prNumber         int
	client           *github.Client
}

func newGitHubClient(event *github.IssueCommentEvent) *gitHubClient {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")})
	tc := oauth2.NewClient(context.Background(), ts)
	c := gitHubClient{
		client:           github.NewClient(tc),
		owner:            *event.GetRepo().Owner.Login,
		repo:             *event.GetRepo().Name,
		prNumber:         *event.GetIssue().Number,
		latestCommitHash: os.Getenv("GITHUB_SHA"),
	}
	return &c
}

func (c *gitHubClient) postComment(comment string) error {
	issueComment := &github.IssueComment{Body: github.String(comment)}
	_, _, err := c.client.Issues.CreateComment(context.Background(), c.owner, c.repo, c.prNumber, issueComment)
	return err
}

func (g *GitHub) PostErr(err string) error {
	if err := g.client.postComment(fmt.Sprintf("%v. Logs: %v", err, g.logLink)); err != nil {
		return errors.Wrap(err, "posting err")
	}
	return nil
}

func (g *GitHub) PostResults(cmps []BenchCmp) error {
	b := bytes.Buffer{}
	Render(&b, cmps, false, false, g.compareTarget)
	return g.client.postComment(formatCommentToMD(b.String()))
}

func formatCommentToMD(rawTable string) string {
	tableContent := strings.Split(rawTable, "\n")
	for i := 0; i <= len(tableContent)-1; i++ {
		e := tableContent[i]
		switch {
		case e == "":

		case strings.Contains(e, "old ns/op"):
			e = "| Benchmark | Old ns/op | New ns/op | Delta |"
			tableContent = append(tableContent[:i+1], append([]string{"|-|-|-|-|"}, tableContent[i+1:]...)...)

		case strings.Contains(e, "old MB/s"):
			e = "| Benchmark | Old MB/s | New MB/s | Speedup |"
			tableContent = append(tableContent[:i+1], append([]string{"|-|-|-|-|"}, tableContent[i+1:]...)...)

		case strings.Contains(e, "old allocs"):
			e = "| Benchmark | Old allocs | New allocs | Delta |"
			tableContent = append(tableContent[:i+1], append([]string{"|-|-|-|-|"}, tableContent[i+1:]...)...)

		case strings.Contains(e, "old bytes"):
			e = "| Benchmark | Old bytes | New bytes | Delta |"
			tableContent = append(tableContent[:i+1], append([]string{"|-|-|-|-|"}, tableContent[i+1:]...)...)

		default:
			// Replace spaces with "|".
			e = strings.Join(strings.Fields(e), "|")
		}
		tableContent[i] = e
	}
	return strings.Join(tableContent, "\n")

}
