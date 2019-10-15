package main

import (
	"database/sql"
	"net/http"

	"github.com/src-d/metadata-retrieval/bbserver"
	"github.com/src-d/metadata-retrieval/database"
	"gopkg.in/src-d/go-cli.v0"
)

// rewritten during the CI build step
var (
	version = "master"
	build   = "dev"
)

var app = cli.New("metadata", version, build, "Bitbucket Server metadata downloader")

func main() {
	app.AddCommand(&Sync{})
	app.RunMain()
}

type Sync struct {
	cli.Command `name:"sync" short-description:"Downloads all the data" long-description:"Downloads all the data"`

	DB       string `long:"db" description:"PostgreSQL URL connection string, e.g. postgres://user:password@127.0.0.1:5432/ghsync?sslmode=disable" required:"true"`
	Version  int    `long:"version" description:"Version tag in the DB" required:"true"`
	Cleanup  bool   `long:"cleanup" description:"Do a garbage collection on the DB, deleting data from other versions"`
	BasePath string `long:"base-path" description:"Bitbucket API base path, e.g. http://localhost:7990/rest" required:"true"`
	Login    string `long:"login" description:"Admin user login" required:"true"`
	Pass     string `long:"pass" description:"Admin user password" required:"true"`
}

func (c *Sync) Execute(args []string) error {
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

	if err = database.Migrate(c.DB); err != nil {
		return err
	}

	httpClient := &http.Client{}
	ctx := bbserver.ContextWithBasicAuth(nil, c.Login, c.Pass)
	d, err := bbserver.NewDownloader(ctx, c.BasePath, httpClient, db)
	if err != nil {
		return err
	}

	projects, err := d.ListProjects()
	if err != nil {
		return err
	}

	for _, p := range projects {
		err := d.DownloadProject(ctx, p, 1)
		if err != nil {
			return err
		}

		repos, err := d.ListRepositories(ctx, p)
		if err != nil {
			return err
		}

		for _, repo := range repos {
			err := d.DownloadRepository(ctx, p, repo, 1)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// func run() error {
// 	// postgres://smacker@127.0.0.1:5432/bbserver?sslmode=disable
// 	connStr := os.Getenv("DB")
// 	// http://localhost:7990/rest
// 	basePath := os.Getenv("BASE_PATH")
// 	// admin
// 	login := os.Getenv("LOGIN")
// 	// admin
// 	pass := os.Getenv("PASSWORD")

// 	db, err := sql.Open("postgres", connStr)
// 	if err != nil {
// 		return err
// 	}

// 	if err = db.Ping(); err != nil {
// 		return err
// 	}

// 	if err = Migrate(connStr); err != nil && err != migrate.ErrNoChange {
// 		return err
// 	}

// 	storer := DB{DB: db}
// 	storer.Version(1)

// 	httpClient := &http.Client{}
// 	ctx := bbserver.ContextWithBasicAuth(nil, login, pass)
// 	bbserver.NewDownloader(basePath, httpClient, db)

// 	err = storer.Begin()
// 	if err != nil {
// 		return fmt.Errorf("could not call Begin(): %v", err)
// 	}

// 	if err := storer.SetActiveVersion(1); err != nil {
// 		return err
// 	}

// 	return storer.Commit()
// }
