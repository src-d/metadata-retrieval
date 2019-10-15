package main

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"

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

	DB      string   `long:"db" description:"PostgreSQL URL connection string, e.g. postgres://user:password@127.0.0.1:5432/ghsync?sslmode=disable"`
	Tokens  []string `long:"tokens" short:"t" env:"GITHUB_TOKENS" env-delim:"," description:"GitHub personal access tokens comma separated" required:"true"`
	Version int      `long:"version" description:"Version tag in the DB"`
	Cleanup bool     `long:"cleanup" description:"Do a garbage collection on the DB, deleting data from other versions"`
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
		func(logger log.Logger, dp *DownloadersPool) error {
			return dp.WithDownloader(func(d *github.Downloader) error {
				return d.DownloadRepository(context.TODO(), c.Owner, c.Name, c.Version)
			})
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
		func(logger log.Logger, dp *DownloadersPool) error {
			return dp.WithDownloader(func(d *github.Downloader) error {
				return d.DownloadOrganization(context.TODO(), c.Name, c.Version)
			})
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
		func(logger log.Logger, dp *DownloadersPool) error {
			err := c.downloadOrg(logger, dp)
			if err != nil {
				return err
			}

			repos, err := c.listRepos(logger, dp)
			if err != nil {
				return err
			}

			return c.downloadRepos(logger, dp, repos)
		})
}

func (c *Ghsync) downloadOrg(logger log.Logger, dp *DownloadersPool) error {
	err := dp.WithDownloader(func(d *github.Downloader) error {
		logger.Infof("downloading organization")
		return d.DownloadOrganization(context.TODO(), c.Name, c.Version)
	})

	if err != nil {
		return fmt.Errorf("failed to download organization %v: %v", c.Name, err)
	}

	logger.Infof("finished downloading organization")
	return nil
}

func (c *Ghsync) listRepos(logger log.Logger, dp *DownloadersPool) ([]string, error) {
	var repos []string
	err := dp.WithDownloader(func(d *github.Downloader) error {
		var err error
		logger.Infof("listing repositories")
		repos, err = d.ListRepositories(context.TODO(), c.Name, c.NoForks)
		return err
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list repositories for organization %v: %v", c.Name, err)
	}

	logger.Infof("found %d repositories", len(repos))
	return repos, nil
}

func (c *Ghsync) downloadRepos(logger log.Logger, dp *DownloadersPool, repos []string) error {
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, dp.Size)

	var done uint64
	for _, repo := range repos {
		wg.Add(1)
		go func(logger log.Logger, r string) {
			defer wg.Done()

			err := dp.WithDownloader(func(d *github.Downloader) error {
				logger.Infof("start downloading '%s'", r)
				return d.DownloadRepository(ctx, c.Name, r, c.Version)
			})

			if ctx.Err() != nil {
				logger.Warningf("stopped due to an error occurred while downloading another repository")
				return
			}

			if err != nil {
				logger.Errorf(err, "error while downloading repository")
				errCh <- fmt.Errorf("error while downloading repository: %v", err)
				logger.Debugf("canceling context to stop running jobs")
				cancel()
				return
			}

			logger.Infof("finished downloading '%s' (%d/%d)", r, atomic.AddUint64(&done, 1), len(repos))
		}(logger.With(log.Fields{"repo": repo}), repo)

		if ctx.Err() != nil {
			break
		}
	}

	wg.Wait()

	select {
	case err := <-errCh:
		return err
	default:
		logger.Infof("finished downloading repositories")
		return nil
	}
}

type bodyFunc = func(logger log.Logger, downloadersPool *DownloadersPool) error

func (c *DownloaderCmd) ExecuteBody(logger log.Logger, fn bodyFunc) error {
	ctx := context.Background()
	var db *sql.DB
	if c.DB == "" {
		log.Infof("using stdout to save the data")
		db = nil
	} else {
		var err error
		db, err = sql.Open("postgres", c.DB)
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

		if err = database.Migrate(c.DB); err != nil {
			return err
		}
	}

	downloadersPool, err := c.buildDownloadersPool(logger, db)
	if err != nil {
		return err
	}

	err = downloadersPool.Begin(ctx)
	if err != nil {
		return err
	}

	err = fn(logger, downloadersPool)
	if err != nil {
		return err
	}

	err = c.commit(ctx, downloadersPool)
	if err != nil {
		return err
	}

	stats, err := downloadersPool.End(ctx)

	logger.With(log.Fields{"total-elapsed": stats.Elapsed}).Infof("all metadata fetched")
	for _, ru := range stats.RatesUsage {
		logger.With(log.Fields{
			"rate-limit-used":  ru.Used,
			"rate-usage-speed": fmt.Sprintf("%f/min", ru.Speed),
		}).Infof("token usage")
	}

	return nil
}

func (c *DownloaderCmd) commit(ctx context.Context, dp *DownloadersPool) error {
	return dp.WithDownloader(func(d *github.Downloader) error {
		var err error
		err = d.SetCurrent(ctx, c.Version)
		if err != nil {
			return err
		}

		if c.Cleanup {
			return d.Cleanup(ctx, c.Version)
		}

		return nil
	})
}

func (c *DownloaderCmd) buildDownloadersPool(logger log.Logger, db *sql.DB) (*DownloadersPool, error) {
	var downloaders []*github.Downloader
	for _, t := range c.Tokens {
		client := oauth2.NewClient(context.TODO(), oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: t},
		))
		if c.LogHTTP {
			setLogTransport(client, logger)
		}

		var d *github.Downloader
		var err error
		if db == nil {
			d, err = github.NewStdoutDownloader(client)
		} else {
			d, err = github.NewDownloader(client, db)
		}

		if err != nil {
			return nil, err
		}

		downloaders = append(downloaders, d)
	}

	return NewDownloadersPool(downloaders)
}
