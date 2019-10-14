package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/src-d/metadata-retrieval/github"
)

type DownloadersPool struct {
	Size    int
	pool    chan *github.Downloader
	started bool
	ended   bool
	t0      time.Time
	stats0  map[*github.Downloader]*downloaderStats
}

type DownloaderPoolStats struct {
	Elapsed    time.Duration
	RatesUsage []*RateUsage
}

type downloaderStats struct {
	Rate int
	Time time.Time
}

type downloaderBuilder = func(c *http.Client) (*github.Downloader, error)

func NewDownloadersPool(downloaders []*github.Downloader) (*DownloadersPool, error) {
	ch := make(chan *github.Downloader, len(downloaders))

	for _, d := range downloaders {
		ch <- d
	}

	return &DownloadersPool{
		Size: len(downloaders),
		pool: ch,
	}, nil
}

func (dp *DownloadersPool) WithDownloader(f func(d *github.Downloader) error) error {
	if !dp.started || dp.ended {
		return fmt.Errorf("invalid state: started=%v, ended=%v",
			dp.started, dp.ended)
	}

	item := <-dp.pool
	defer func() {
		dp.pool <- item
	}()

	return f(item)
}

func (dp *DownloadersPool) Begin(ctx context.Context) error {
	if dp.started || dp.ended {
		return fmt.Errorf("invalid state for `Begin()`: started=%v, ended=%v",
			dp.started, dp.ended)
	}

	dp.started = true
	dp.t0 = time.Now()
	stats0, err := dp.stats(ctx)
	if err != nil {
		return err
	}

	dp.stats0 = stats0
	return nil
}

func (dp *DownloadersPool) End(ctx context.Context) (*DownloaderPoolStats, error) {
	if !dp.started || dp.ended {
		return nil, fmt.Errorf("invalid state for `End()`: started=%v, ended=%v",
			dp.started, dp.ended)
	}

	t1 := time.Now()

	stats1, err := dp.stats(ctx)
	dp.ended = true
	if err != nil {
		return nil, err
	}

	return dp.calculateStats(t1, stats1)
}

// calculateStats returns elapsed time and the usage of the api.
//
// NB: this return incorrect result for api usage if a rate reset occurs between
// `Begin()` and `End()`.
func (dp *DownloadersPool) calculateStats(t1 time.Time, stats1 map[*github.Downloader]*downloaderStats) (*DownloaderPoolStats, error) {
	var rateUsages []*RateUsage

	elapsed := t1.Sub(dp.t0)
	for d, s0 := range dp.stats0 {
		s1, ok := stats1[d]
		if !ok {
			return nil, fmt.Errorf("cannot find stats for downloader")
		}

		used := s0.Rate - s1.Rate
		rateUsages = append(rateUsages, &RateUsage{
			Used:  used,
			Speed: float64(used) / elapsed.Minutes(),
		})
	}

	return &DownloaderPoolStats{
		Elapsed:    elapsed,
		RatesUsage: rateUsages,
	}, nil
}

func (dp *DownloadersPool) stats(ctx context.Context) (map[*github.Downloader]*downloaderStats, error) {
	stats := make(map[*github.Downloader]*downloaderStats)
	for i := 0; i < dp.Size; i++ {
		err := dp.WithDownloader(func(d *github.Downloader) error {
			dStats, err := dp.singleStats(ctx, d)
			if err != nil {
				return err
			}

			stats[d] = dStats
			return nil
		})

		if err != nil {
			return nil, err
		}
	}

	return stats, nil
}

func (dp *DownloadersPool) singleStats(ctx context.Context, d *github.Downloader) (*downloaderStats, error) {
	rate, err := d.RateRemaining(ctx)
	if err != nil {
		return nil, err
	}

	return &downloaderStats{
		Rate: rate,
		Time: time.Now(),
	}, nil
}

type RateUsage struct {
	Used  int
	Speed float64
}
