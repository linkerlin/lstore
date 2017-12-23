package lstore

import (
	"unsafe"
	"sync/atomic"
	"github.com/v2pro/plz/concurrent"
	"context"
	"github.com/v2pro/plz/countlog"
	"path"
)

const TailSegmentFileName = "tail.segment"

type Store struct {
	Directory          string
	CommandQueueSize   int
	TailSegmentMaxSize int64
	currentVersion     unsafe.Pointer
	commandQueue       chan Command
	executor           *concurrent.UnboundedExecutor
}

type Command func(store *StoreVersion) *StoreVersion

type StoreVersion struct {
	referenceCounter uint32
	tail             *Segment
}

func (store *Store) Start() error {
	if store.CommandQueueSize == 0 {
		store.CommandQueueSize = 1024
	}
	if store.Directory == "" {
		store.Directory = "/tmp"
	}
	if store.TailSegmentMaxSize == 0 {
		store.TailSegmentMaxSize = 200 * 1024 * 1024
	}
	store.commandQueue = make(chan Command, store.CommandQueueSize)
	initialVersion, err := store.loadData()
	if err != nil {
		return err
	}
	atomic.StorePointer(&store.currentVersion, unsafe.Pointer(initialVersion))
	store.startCommandQueue(initialVersion)
	return nil
}

func (store *Store) loadData() (*StoreVersion, error) {
	segment, err := openSegment(path.Join(store.Directory, TailSegmentFileName), store.TailSegmentMaxSize)
	if err != nil {
		return nil, err
	}
	return &StoreVersion{referenceCounter: 1, tail: segment}, nil
}

func (store *Store) startCommandQueue(initialVersion *StoreVersion) {
	store.executor = concurrent.NewUnboundedExecutor()
	store.executor.Go(func(ctx context.Context) {
		currentVersion := initialVersion
		defer func() {
			err := currentVersion.Close()
			if err != nil {
				countlog.Error("event!store.failed to close", "err", err)
			}
		}()
		for {
			var command Command
			select {
			case <-ctx.Done():
				return
			case command = <-store.commandQueue:
			}
			newVersion := handleCommand(command, currentVersion)
			if newVersion != nil {
				err := currentVersion.Close()
				if err != nil {
					countlog.Error("event!store.failed to close", "err", err)
				}
				currentVersion = newVersion
				atomic.StorePointer(&store.currentVersion, unsafe.Pointer(currentVersion))
			}
		}
	})
}

func handleCommand(command Command, currentVersion *StoreVersion) *StoreVersion {
	defer func() {
		recovered := recover()
		if recovered != nil && recovered != concurrent.StopSignal {
			countlog.Fatal("event!store.panic",
				"err", recovered,
				"stacktrace", countlog.ProvideStacktrace)
		}
	}()
	return command(currentVersion)
}

func (store *Store) Stop(ctx context.Context) {
	store.executor.StopAndWait(ctx)
}

func (store *Store) Latest() *StoreVersion {
	for {
		store := (*StoreVersion)(atomic.LoadPointer(&store.currentVersion))
		if store == nil {
			return nil
		}
		counter := atomic.LoadUint32(&store.referenceCounter)
		if counter == 0 {
			// retry
			continue
		}
		if !atomic.CompareAndSwapUint32(&store.referenceCounter, counter, counter+1) {
			// retry
			continue
		}
		return store
	}
}

func (store *Store) AsyncExecute(ctx context.Context, cmd Command) {
	select {
	case store.commandQueue <- cmd:
	case <-ctx.Done():
	}
}

func (store *StoreVersion) Tail() *Segment {
	return store.tail
}

func (store *StoreVersion) Close() error {
	if !store.decreaseReference() {
		return nil // still in use
	}
	return store.tail.Close()
}

func (store *StoreVersion) decreaseReference() bool {
	for {
		counter := atomic.LoadUint32(&store.referenceCounter)
		if counter == 0 {
			return true
		}
		if atomic.CompareAndSwapUint32(&store.referenceCounter, counter, counter-1) {
			return counter == 1 // last one should close the store
		}
	}
}
