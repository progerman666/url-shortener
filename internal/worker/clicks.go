package worker

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/progerman666/url-shortener/internal/repository"
)

type ClickEvent struct {
	LinkID    int64
	UserAgent string
	IP        string
}

type ClickWorker struct {
	ch   chan ClickEvent
	repo *repository.LinkRepository
	wg   sync.WaitGroup
}

func NewClickWorker(repo *repository.LinkRepository, bufferSize int) *ClickWorker {
	return &ClickWorker{
		ch:   make(chan ClickEvent, bufferSize),
		repo: repo,
	}
}

func (w *ClickWorker) Start() {
	w.wg.Add(1)
	go w.run()
}

// Enqueue не блокируется: если буфер полон — возвращает false.
func (w *ClickWorker) Enqueue(e ClickEvent) bool {
	select {
	case w.ch <- e:
		return true
	default:
		return false
	}
}

// Stop закрывает канал и ждёт, пока воркер допишет всё в БД.
func (w *ClickWorker) Stop() {
	close(w.ch)
	w.wg.Wait()
}

func (w *ClickWorker) run() {
	defer w.wg.Done()

	const (
		flushInterval = 2 * time.Second
		batchSize     = 100
	)

	batch := make([]ClickEvent, 0, batchSize)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, e := range batch {
			if err := w.repo.InsertClick(ctx, e.LinkID, e.UserAgent, e.IP); err != nil {
				log.Printf("worker: insert click (link=%d): %v", e.LinkID, err)
			}
		}
		log.Printf("worker: flushed %d clicks", len(batch))
		batch = batch[:0]
	}

	for {
		select {
		case e, ok := <-w.ch:
			if !ok {
				flush() // канал закрыт — последний сброс
				return
			}
			batch = append(batch, e)
			if len(batch) >= batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}