package handler

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Deliberately independent of the gocheck-based mainSuite fixtures in the
// rest of this package (SetUpTest/TearDownTest bring up a real sqlite DB,
// vips, and OS-level test directories) -- this only exercises the in-memory
// channel bookkeeping added for the preview request queue, so it can run on
// its own via `go test -run TestAcquirePreviewSlot`.

func newTestPreviewHandler(maxConcurrent uint) *Handler {
	return &Handler{
		previewCh:     make(chan struct{}, maxConcurrent),
		previewWaitCh: make(chan struct{}, previewQueueDepth(maxConcurrent)),
	}
}

// A burst that exactly fills the wait room (queue depth) must all be
// admitted -- none rejected -- while still never letting more than
// maxConcurrent of them "generate" at once.
func TestAcquirePreviewSlotRespectsConcurrencyCap(t *testing.T) {
	const maxConcurrent = 3
	h := newTestPreviewHandler(maxConcurrent)
	requests := int(previewQueueDepth(maxConcurrent))

	var active int32
	var maxObserved int32
	var rejected int32
	var wg sync.WaitGroup

	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !h.acquirePreviewSlot() {
				atomic.AddInt32(&rejected, 1)
				return
			}
			defer h.releasePreviewSlot()

			n := atomic.AddInt32(&active, 1)
			for {
				old := atomic.LoadInt32(&maxObserved)
				if n <= old || atomic.CompareAndSwapInt32(&maxObserved, old, n) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			atomic.AddInt32(&active, -1)
		}()
	}
	wg.Wait()

	if rejected != 0 {
		t.Fatalf("got %d rejected requests, want 0 -- a burst that exactly fills the wait room should all be admitted", rejected)
	}
	if maxObserved > maxConcurrent {
		t.Fatalf("observed %d requests generating previews concurrently, want <= %d", maxObserved, maxConcurrent)
	}
}

// Once both the generation slots and the wait room behind them are full,
// the next caller must be rejected immediately (not block indefinitely) --
// this is the whole point of the wait room: bound how many goroutines can
// be alive waiting for a preview, instead of accepting unbounded requests
// and blocking their goroutines (and the memory they hold) forever.
func TestAcquirePreviewSlotRejectsOnceWaitRoomIsFull(t *testing.T) {
	const maxConcurrent = 2
	h := newTestPreviewHandler(maxConcurrent)
	depth := int(previewQueueDepth(maxConcurrent))

	release := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < maxConcurrent+depth; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !h.acquirePreviewSlot() {
				return
			}
			defer h.releasePreviewSlot()
			<-release
		}()
	}
	// Let every goroutine reach either "generating" or "waiting in line" --
	// pure in-memory channel operations, so this is a generous margin.
	time.Sleep(200 * time.Millisecond)

	if h.acquirePreviewSlot() {
		h.releasePreviewSlot()
		t.Fatal("expected acquirePreviewSlot to reject once generation slots + wait room are both full, but it was granted")
	}

	close(release)
	wg.Wait()
}

// A request that gets rejected (wait room full) must not have consumed a
// generation slot -- otherwise rejections would themselves leak capacity.
func TestAcquirePreviewSlotRejectionDoesNotConsumeAGenerationSlot(t *testing.T) {
	const maxConcurrent = 1
	h := newTestPreviewHandler(maxConcurrent)
	depth := int(previewQueueDepth(maxConcurrent))

	release := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < maxConcurrent+depth; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if h.acquirePreviewSlot() {
				defer h.releasePreviewSlot()
				<-release
			}
		}()
	}
	time.Sleep(200 * time.Millisecond)

	if h.acquirePreviewSlot() {
		t.Fatal("expected rejection while the system is saturated")
		h.releasePreviewSlot()
	}
	close(release)
	wg.Wait()

	// Now that everything has drained, a fresh request must be granted
	// immediately -- if a rejected request had wrongly consumed a slot,
	// this would hang.
	done := make(chan struct{})
	go func() {
		if !h.acquirePreviewSlot() {
			t.Error("expected a slot to be available after the system fully drained")
		} else {
			h.releasePreviewSlot()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("acquirePreviewSlot hung -- a generation slot appears to have leaked")
	}
}
