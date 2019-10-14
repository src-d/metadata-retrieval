package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

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

	Orgs    []string `long:"orgs" env:"GHSYNC_ORGS" env-delim:"," description:"GitHub organizations names comma separated" required:"true"`
	NoForks bool     `long:"no-forks"  env:"GHSYNC_NO_FORKS" description:"github forked repositories will be skipped"`
}

func (c *Ghsync) Execute(args []string) error {
	return c.ExecuteBody(
		log.DefaultLogger,
		func(logger log.Logger, dp *DownloadersPool) error {
			repos, err := c.listAllRepos(logger, dp, c.Orgs)
			if err != nil {
				return err
			}

			err = c.downloadOrgs(logger, dp, c.Orgs)
			if err != nil {
				return err
			}

			return c.downloadRepos(logger, dp, repos)
		})
}

func (c *Ghsync) listAllRepos(logger log.Logger, dp *DownloadersPool, orgs []string) ([]string, error) {
	var repos []string
	logger.Infof("listing all repositories")
	for _, org := range orgs {
		orgRepos, err := c.listRepos(logger.With(log.Fields{"organization": org}), dp, org)
		if err != nil {
			return nil, err
		}

		for _, r := range orgRepos {
			repos = append(repos, fmt.Sprintf("%s/%s", org, r))
		}
	}

	logger.Infof("found %d repositories in total", len(repos))
	return repos, nil
}

func (c *Ghsync) listRepos(logger log.Logger, dp *DownloadersPool, org string) ([]string, error) {
	var repos []string
	err := dp.WithDownloader(func(d *github.Downloader) error {
		var err error
		logger.Infof("listing repositories")
		repos, err = d.ListRepositories(context.TODO(), org, c.NoForks)
		return err
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list repositories for organization %v: %v", org, err)
	}

	logger.Infof("found %d repositories", len(repos))
	return repos, nil
}

func (c *Ghsync) downloadOrgs(logger log.Logger, dp *DownloadersPool, orgs []string) error {
	resourceType := "org"

	downloadFn := func(ctx context.Context, d *github.Downloader, org string) error {
		return d.DownloadOrganization(ctx, org, c.Version)
	}

	prepareLoggerFn := func(logger log.Logger, repo string) log.Logger {
		return logger
	}

	return c.downloadParallel(logger, dp, resourceType, orgs, downloadFn, prepareLoggerFn)
}

func (c *Ghsync) downloadRepos(logger log.Logger, dp *DownloadersPool, repos []string) error {
	resourceType := "repo"

	downloadFn := func(ctx context.Context, d *github.Downloader, repo string) error {
		splitted := strings.Split(repo, "/")
		return d.DownloadRepository(ctx, splitted[0], splitted[1], c.Version)
	}

	prepareLoggerFn := func(logger log.Logger, repo string) log.Logger {
		splitted := strings.Split(repo, "/")
		return logger.With(log.Fields{"owner": splitted[0], "repo": splitted[1]})
	}

	return c.downloadParallel(logger, dp, resourceType, repos, downloadFn, prepareLoggerFn)
}

func (c *Ghsync) downloadParallel(
	logger log.Logger,
	dp *DownloadersPool,
	resourceType string,
	params []string,
	downloadFn func(ctx context.Context, d *github.Downloader, param string) error,
	prepareLoggerFn func(logger log.Logger, param string) log.Logger,
) error {
	logger = logger.With(log.Fields{"resource-type": resourceType})
	logger.Infof("started downloading all %ss", resourceType)

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, dp.Size)

	var done uint64
	for _, p := range params {
		wg.Add(1)

		logger = prepareLoggerFn(logger.With(log.Fields{resourceType: p}), p)

		go func(logger log.Logger, p string) {
			defer wg.Done()

			err := dp.WithDownloader(func(d *github.Downloader) error {
				logger.Infof("start downloading '%s'", p)
				return downloadFn(ctx, d, p)
			})

			if ctx.Err() != nil {
				logger.Warningf("stopped due to an error occurred while downloading another %s", resourceType)
				return
			}

			if err != nil {
				logger.Errorf(err, "error while downloading %s", resourceType)
				errCh <- fmt.Errorf("error while downloading %s: %v", resourceType, err)
				logger.Debugf("canceling context to stop running jobs")
				cancel()
				return
			}

			logger.Infof("finished downloading '%s' (%d/%d)", p, atomic.AddUint64(&done, 1), len(params))
		}(logger, p)

		if ctx.Err() != nil {
			break
		}
	}

	wg.Wait()

	select {
	case err := <-errCh:
		return err
	default:
		logger.Infof("finished downloading all %ss", resourceType)
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

		if err = database.Migrate(c.DB); err != nil && err != migrate.ErrNoChange {
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
