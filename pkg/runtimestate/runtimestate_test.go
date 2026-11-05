package runtimestate

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMapConcurrentAccess(t *testing.T) {
	m := New[string, int]()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := "k"
			if i%2 == 0 {
				key = "j"
			}
			m.Set(key, i)
			m.Get(key)
			m.Load(key)
			m.CompareAndDelete(key, i)
			m.Delete(key)
		}(i)
	}
	wg.Wait()
}

func TestCompareAndDeleteKeepsNewerValue(t *testing.T) {
	type client struct{ name string }
	m := New[string, *client]()
	old := &client{"old"}
	newer := &client{"new"}

	m.Set("a", old)
	m.Set("a", newer)

	if m.CompareAndDelete("a", old) {
		t.Fatal("deleted the newer client while cleaning up the old one")
	}
	if m.Get("a") != newer {
		t.Fatal("newer client should still be registered")
	}
	if !m.CompareAndDelete("a", newer) {
		t.Fatal("expected the newer client to be deleted")
	}
}

func TestLockKeySerializesSameKey(t *testing.T) {
	m := New[string, int]()
	var inside int32
	var maxInside int32
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := m.LockKey("instance")
			defer unlock()
			n := atomic.AddInt32(&inside, 1)
			for {
				cur := atomic.LoadInt32(&maxInside)
				if n <= cur || atomic.CompareAndSwapInt32(&maxInside, cur, n) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			atomic.AddInt32(&inside, -1)
		}()
	}
	wg.Wait()

	if maxInside != 1 {
		t.Fatalf("expected one holder at a time, got %d", maxInside)
	}
	if m.guards != nil && len(m.guards) != 0 {
		t.Fatalf("guards should be released, got %d", len(m.guards))
	}
}

func TestLockKeyDoesNotBlockOtherKeys(t *testing.T) {
	m := New[string, int]()
	unlockA := m.LockKey("a")
	defer unlockA()

	done := make(chan struct{})
	go func() {
		unlockB := m.LockKey("b")
		unlockB()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("lock on key a blocked key b")
	}
}

func TestLockKeyUnlockIsIdempotent(t *testing.T) {
	m := New[string, int]()
	unlock := m.LockKey("a")
	unlock()
	unlock()

	unlockAgain := m.LockKey("a")
	unlockAgain()
}

func TestSessionStopIsIdempotentAndKeepsFirstReason(t *testing.T) {
	s := NewSession()
	if s.IsStopping() {
		t.Fatal("new session should not be stopping")
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Stop("later")
		}()
	}
	wg.Wait()
	s.Stop("ignored")

	if !s.IsStopping() {
		t.Fatal("session should be stopping")
	}
	if s.Reason() != "later" {
		t.Fatalf("reason = %q", s.Reason())
	}
}

func TestSessionStopReachesEveryListener(t *testing.T) {
	s := NewSession()
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-s.Stopping()
		}()
	}

	s.Stop("test")

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("not every listener saw the stop")
	}
}

func TestSessionStopAndWait(t *testing.T) {
	s := NewSession()
	go func() {
		<-s.Stopping()
		time.Sleep(10 * time.Millisecond)
		s.Finish()
	}()

	if !s.StopAndWait("test", time.Second) {
		t.Fatal("StopAndWait timed out")
	}

	stuck := NewSession()
	if stuck.StopAndWait("test", 10*time.Millisecond) {
		t.Fatal("expected timeout when cleanup never finishes")
	}
}

func TestNilSessionIsSafe(t *testing.T) {
	var s *Session
	s.Stop("x")
	s.Finish()
	if !s.IsStopping() {
		t.Fatal("nil session should report stopping")
	}
	if !s.StopAndWait("x", time.Millisecond) {
		t.Fatal("nil session should not block")
	}
}
