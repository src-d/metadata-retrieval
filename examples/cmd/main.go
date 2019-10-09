package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/src-d/metadata-retrieval/database"
	"github.com/src-d/metadata-retrieval/github"
	"golang.org/x/oauth2"
	"gopkg.in/src-d/go-cli.v0"
	"gopkg.in/src-d/go-log.v1"
)

// rewritten during the CI build step
var (
	version = "master"
	build   = "dev"
)

var app = cli.New("metadata", version, build, "GitHub metadata downloader")

func main() {
	app.AddCommand(&Repository{})
	app.AddCommand(&Organization{})
	app.AddCommand(&Ghsync{})
	app.RunMain()
}

type DownloaderCmd struct {
	LogHTTP bool `long:"log-http" description:"log http requests (debug level)"`

	DB      string `long:"db" description:"PostgreSQL URL connection string, e.g. postgres://user:password@127.0.0.1:5432/ghsync?sslmode=disable"`
	Token   string `long:"token" short:"t" env:"GITHUB_TOKEN" description:"GitHub personal access token" required:"true"`
	Version int    `long:"version" description:"Version tag in the DB"`
	Cleanup bool   `long:"cleanup" description:"Do a garbage collection on the DB, deleting data from other versions"`
}

type Repository struct {
	cli.Command `name:"repo" short-description:"Download metadata for a GitHub repository" long-description:"Download metadata for a GitHub repository"`
	DownloaderCmd

	Owner string `long:"owner"  required:"true"`
	Name  string `long:"name"  required:"true"`
}

func (c *Repository) Execute(args []string) error {
	return c.ExecuteBody(
		log.New(log.Fields{"owner": c.Owner, "repo": c.Name}),
		func(logger log.Logger, httpClient *http.Client, downloader *github.Downloader) error {
			return downloader.DownloadRepository(context.TODO(), c.Owner, c.Name, c.Version)
		})
}

type Organization struct {
	cli.Command `name:"org" short-description:"Download metadata for a GitHub organization" long-description:"Download metadata for a GitHub organization"`
	DownloaderCmd

	Name string `long:"name" description:"GitHub organization name" required:"true"`
}

func (c *Organization) Execute(args []string) error {
	return c.ExecuteBody(
		log.New(log.Fields{"org": c.Name}),
		func(logger log.Logger, httpClient *http.Client, downloader *github.Downloader) error {
			return downloader.DownloadOrganization(context.TODO(), c.Name, c.Version)
		})
}

type Ghsync struct {
	cli.Command `name:"ghsync" short-description:"Mimics ghsync deep command" long-description:"Mimics ghsync deep command"`
	DownloaderCmd

	Name    string `long:"name" description:"GitHub organization name" required:"true"`
	NoForks bool   `long:"no-forks"  env:"GHSYNC_NO_FORKS" description:"github forked repositories will be skipped"`
}

func (c *Ghsync) Execute(args []string) error {
	return c.ExecuteBody(
		log.New(log.Fields{"org": c.Name}),
		func(logger log.Logger, httpClient *http.Client, downloader *github.Downloader) error {
			repos, err := listRepositories(context.TODO(), httpClient, c.Name, c.NoForks)
			if err != nil {
				return err
			}

			err = downloader.DownloadOrganization(context.TODO(), c.Name, c.Version)
			if err != nil {
				return fmt.Errorf("failed to download organization %v: %v", c.Name, err)
			}

			for i, repo := range repos {
				logger.Infof("start downloading '%s'", repo)
				err = downloader.DownloadRepository(context.TODO(), c.Name, repo, c.Version)
				if err != nil {
					return fmt.Errorf("failed to download repository %v/%v: %v", c.Name, repo, err)
				}
				logger.Infof("finished downloading '%s' (%d/%d)", repo, i+1, len(repos))
			}

			return nil

		})
}

type bodyFunc = func(logger log.Logger, httpClient *http.Client, downloader *github.Downloader) error

func (c *DownloaderCmd) ExecuteBody(logger log.Logger, fn bodyFunc) error {
	client := oauth2.NewClient(context.TODO(), oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: c.Token},
	))

	if c.LogHTTP {
		setLogTransport(client, logger)
	}

	var downloader *github.Downloader
	if c.DB == "" {
		log.Infof("using stdout to save the data")
		var err error
		downloader, err = github.NewStdoutDownloader(client)
		if err != nil {
			return err
		}
	} else {
		db, err := sql.Open("postgres", c.DB)
		if err != nil {
			return err
		}

		defer func() {
			if err != nil {
				db.Close()
				db = nil
			}
		}()

		if err = db.Ping(); err != nil {
			return err
		}

		if err = database.Migrate(c.DB); err != nil && err != migrate.ErrNoChange {
			return err
		}

		downloader, err = github.NewDownloader(client, db)
	}

	rate0, err := downloader.RateRemaining(context.TODO())
	if err != nil {
		return err
	}
	t0 := time.Now()

	err = fn(logger, client, downloader)
	if err != nil {
		return err
	}

	err = downloader.SetCurrent(c.Version)
	if err != nil {
		return err
	}

	if c.Cleanup {
		return downloader.Cleanup(c.Version)
	}

	elapsed := time.Since(t0)

	rate1, err := downloader.RateRemaining(context.TODO())
	if err != nil {
		return err
	}
	rateUsed := rate0 - rate1

	logger.With(log.Fields{"rate-limit-used": rateUsed, "total-elapsed": elapsed}).Infof("All metadata fetched")

	return nil
}
